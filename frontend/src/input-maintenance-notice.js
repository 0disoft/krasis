import { nativeText } from './native-i18n.js';

const refreshKeys = {ok:'maintenance_done',input_refresh_cancelled:'maintenance_cancelled',input_refresh_pending:'maintenance_pending',input_refresh_failed:'maintenance_failed',busy:'maintenance_busy'};
const reconnectKeys = {ok:'maintenance_requested',not_attempted:'maintenance_not_attempted',outcome_unknown:'maintenance_unknown',cancelled:'maintenance_cancelled',stale_peer:'maintenance_peer_changed',invalid_peer:'maintenance_peer_changed'};

export function inputSetupNotice(value, busy = false) {
  const state = busy ? 'UPDATING' : value?.inputSetup;
  if (['UPDATING','PENDING'].includes(state)) return {key:'input_setup_' + state, action:''};
  if (['NOT_INSTALLED','UPDATE_REQUIRED','CANCELLED','FAILED'].includes(state)) {
    return {key:'input_setup_' + state, action:state === 'NOT_INSTALLED' ? 'input_setup' : 'input_refresh'};
  }
  if (state === 'READY') {
    return value?.inputBackend === 'PORTABLE' ? {key:'input_portable', action:'input_refresh'} : {key:'', action:''};
  }
  // Unknown installation health is not evidence of ordinary input or a reason
  // to start the installer. Only native startup's preparing code means waiting.
  return {key:value?.code === 'preparing' ? 'input_setup_UNKNOWN' : 'input_setup_unavailable', action:''};
}

export function inputMaintenanceNotice(value, locale) {
  if (value?.reconnectCode === 'input_refresh_wait_expired') return nativeText(locale,'input_refresh_wait_expired');
  if (value?.reconnectCode === 'input_refresh_restore_failed') return nativeText(locale,'input_refresh_restore_failed');
  if (!value?.inputRefreshCode && !value?.reconnectCode) {
    const key = {input_refresh_cancelled:'input_last_cancelled',input_refresh_failed:'input_last_failed'}[value?.inputAttemptCode];
    return key && ['UPDATE_REQUIRED','NOT_INSTALLED','UNKNOWN'].includes(value?.inputSetup) ? nativeText(locale,key) : '';
  }
  if (value.inputRefreshCode === 'ok' && ['ok','not_attempted'].includes(value.reconnectCode)) return '';
  return nativeText(locale,'maintenance_outcome')
    .replace('{refresh}',nativeText(locale,refreshKeys[value.inputRefreshCode] ?? 'maintenance_unknown'))
    .replace('{reconnect}',nativeText(locale,reconnectKeys[value.reconnectCode] ?? 'maintenance_unknown'));
}
