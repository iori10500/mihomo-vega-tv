import type {KeplerTurboModule} from '@amazon-devices/keplerscript-turbomodule-api';
import {TurboModuleRegistry} from '@amazon-devices/keplerscript-turbomodule-api';

export interface Spec extends KeplerTurboModule {
  setRequested(value: boolean): string;
  requested(): boolean;
  routingMode(): string;
  setRoutingMode(mode: string): string;
  serviceChoices(): string;
  setServiceChoices(choices: string): string;
  manage(request: string): string;
  memorySample(): string;
  routeReport(): string;
  proxyHost(): string;
  startCore(): string;
  stopCore(): string;
  coreState(): string;
  saveConfig(json: string): string;
}

export default TurboModuleRegistry.getEnforcing<Spec>('Core');
