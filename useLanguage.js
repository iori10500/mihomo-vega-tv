import {useEffect, useRef, useState} from 'react';
import {AppState, AsyncStorage, I18nManager} from 'react-native';
import {resolveLanguage} from './i18n';

const key = 'ui-language';

function tvLocale() {
  return I18nManager.getSystemLocale() || I18nManager.getConstants().localeIdentifier;
}

export function useLanguage() {
  const [systemLocale, setSystemLocale] = useState(tvLocale);
  const [preference, setPreference] = useState('system');
  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');
  const saving = useRef(false);

  useEffect(() => {
    let mounted = true;
    AsyncStorage.getItem(key).then(value => {
      if (mounted && (value === 'en' || value === 'zh')) setPreference(value);
    }).catch(e => {
      if (mounted) setError(e.message);
    }).finally(() => {
      if (mounted) setReady(true);
    });
    const refresh = () => setSystemLocale(tvLocale());
    const app = AppState.addEventListener('change', state => {
      if (state === 'active') refresh();
    });
    const locale = I18nManager.addEventListener(I18nManager.SettingEventName.Locale, refresh);
    return () => {
      mounted = false;
      app.remove();
      locale?.remove();
    };
  }, []);

  async function select(value) {
    if (!ready || saving.current) return;
    saving.current = true;
    try {
      await AsyncStorage.setItem(key, value);
      setPreference(value);
      setError('');
    } catch (e) {
      setError(e.message);
    } finally {
      saving.current = false;
    }
  }

  return {language: resolveLanguage(preference, systemLocale), preference, ready, select, error};
}
