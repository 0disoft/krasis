import { nativeText } from './native-i18n.js';

const refreshKeys = {ok:'maintenance_done',input_refresh_cancelled:'maintenance_cancelled',input_refresh_pending:'maintenance_pending',input_refresh_failed:'maintenance_failed',busy:'maintenance_busy'};
const reconnectKeys = {ok:'maintenance_requested',not_attempted:'maintenance_not_attempted',outcome_unknown:'maintenance_unknown',cancelled:'maintenance_cancelled',stale_peer:'maintenance_peer_changed',invalid_peer:'maintenance_peer_changed'};

export function inputMaintenanceNotice(value, locale) {
  if (!value?.inputRefreshCode && !value?.reconnectCode) return '';
  if (value.inputRefreshCode === 'ok' && ['ok','not_attempted'].includes(value.reconnectCode)) return '';
  return nativeText(locale,'maintenance_outcome')
    .replace('{refresh}',nativeText(locale,refreshKeys[value.inputRefreshCode] ?? 'maintenance_unknown'))
    .replace('{reconnect}',nativeText(locale,reconnectKeys[value.reconnectCode] ?? 'maintenance_unknown'));
}
