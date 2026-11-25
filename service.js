import {measureMemory} from './memory';
let endMeasure;
import {HeadlessEntryPointRegistry} from '@amazon-devices/headless-task-manager';
import Core from './src/NativeCore';
let timer;
let enabled = false;
function reconcile() {
  const wanted = Core.requested();
  if (wanted === enabled) return;
  const error = wanted ? Core.startCore() : Core.stopCore();
  if (error) {
    if (wanted) {
      const reset = Core.setRequested(false);
      if (reset) throw Error(reset);
    }
    throw Error(error);
  }
  enabled = wanted;
  console.info(wanted ? 'MihomoTV service connected' : 'MihomoTV service disconnected');
}
HeadlessEntryPointRegistry.registerHeadlessEntryPoint('com.iori.mihomo.service::onStartService', () => async () => {
  if (!endMeasure) endMeasure=measureMemory('vpn');
  reconcile();
  if (!timer) timer = setInterval(() => {try {reconcile();} catch (e) {console.warn('MihomoTV service: ' + e.message);}}, 1500);
  return true;
});
HeadlessEntryPointRegistry.registerHeadlessEntryPoint('com.iori.mihomo.service::onStopService', () => async () => {
  if (endMeasure) endMeasure();
  endMeasure=null;
  clearInterval(timer);
  timer = null;
  if (enabled) Core.stopCore();
  enabled = false;
  return true;
});
