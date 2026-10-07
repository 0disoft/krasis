import test from 'node:test';
import assert from 'node:assert/strict';
import { inputMaintenanceNotice } from './input-maintenance-notice.js';

test('successful refreshes stay quiet and combined failures keep both outcomes', () => {
  assert.equal(inputMaintenanceNotice({},'ko'),'');
  for (const reconnectCode of ['ok','not_attempted']) {
    assert.equal(inputMaintenanceNotice({inputRefreshCode:'ok',reconnectCode},'ko'),'');
  }
  const value = {inputRefreshCode:'input_refresh_failed',reconnectCode:'outcome_unknown'};
  assert.equal(inputMaintenanceNotice(value,'ko'),'갱신: 실패 · 재연결: 확인 필요');
  assert.equal(inputMaintenanceNotice(value,'en'),'Refresh: failed · reconnect: needs verification');
  assert.equal(inputMaintenanceNotice({inputRefreshCode:'input_refresh_cancelled',reconnectCode:'ok'},'ko'),'갱신: 취소됨 · 재연결: 요청 처리됨');
  assert.equal(inputMaintenanceNotice({inputRefreshCode:'input_refresh_pending',reconnectCode:'not_attempted'},'ko'),'갱신: 결과 확인 필요 · 재연결: 시도하지 않음');
});

test('unconfirmed values never show raw details or claim success', () => {
  const value = {inputRefreshCode:'/private/path',reconnectCode:'private error'};
  assert.equal(inputMaintenanceNotice(value,'ko'),'갱신: 확인 필요 · 재연결: 확인 필요');
  assert.equal(inputMaintenanceNotice({inputRefreshCode:'ok',reconnectCode:'stale_peer'},'ko'),'갱신: 완료 · 재연결: 기기 다시 선택 필요');
});
