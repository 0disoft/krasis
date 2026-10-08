import test from 'node:test';
import assert from 'node:assert/strict';
import { inputMaintenanceNotice, inputSetupNotice } from './input-maintenance-notice.js';
test('historical installation failures remain visible only where current health needs attention', () => {
  assert.equal(inputMaintenanceNotice({inputAttemptCode:'input_refresh_cancelled',inputSetup:'UPDATE_REQUIRED'},'ko'),'최근 보호 입력 설치는 취소됐습니다.');
  assert.equal(inputMaintenanceNotice({inputAttemptCode:'input_refresh_failed',inputSetup:'UNKNOWN'},'en'),'The last protected input installation failed.');
  for (const inputSetup of ['READY','UPDATING','PENDING','FAILED','CANCELLED']) {
    assert.equal(inputMaintenanceNotice({inputAttemptCode:'input_refresh_failed',inputSetup},'ko'),'');
  }
  assert.equal(inputMaintenanceNotice({inputAttemptCode:'private path',inputSetup:'UNKNOWN'},'ko'),'');
});

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

test('expired automatic confirmation remains distinct from the installation outcome', () => {
  for (const inputRefreshCode of ['ok','input_refresh_pending','input_refresh_failed']) {
    const value = {inputRefreshCode,reconnectCode:'input_refresh_wait_expired'};
    assert.equal(inputMaintenanceNotice(value,'ko'),'자동 확인 시간이 끝났습니다. 새로고침으로 상태를 확인한 뒤 연결해 주세요.');
    assert.equal(inputMaintenanceNotice(value,'en'),'Automatic checking has timed out. Refresh to check the status, then connect.');
  }
});

test('unknown health is visible without offering installation or claiming ordinary input', () => {
  for (const inputBackend of ['UNKNOWN','PORTABLE','PROTECTED']) {
    assert.deepEqual(inputSetupNotice({inputSetup:'UNKNOWN',inputBackend,code:'preparing'}), {key:'input_setup_UNKNOWN',action:''});
    for (const code of ['ok','unavailable','busy']) {
      assert.deepEqual(inputSetupNotice({inputSetup:'UNKNOWN',inputBackend,code}), {key:'input_setup_unavailable',action:''});
    }
  }
  assert.deepEqual(inputSetupNotice({}), {key:'input_setup_unavailable',action:''});
  assert.deepEqual(inputSetupNotice({inputSetup:'/private/path',code:'ok'}), {key:'input_setup_unavailable',action:''});
  assert.deepEqual(inputSetupNotice({inputSetup:'READY',inputBackend:'PORTABLE'}), {key:'input_portable',action:'input_refresh'});
  assert.deepEqual(inputSetupNotice({inputSetup:'READY',inputBackend:'PROTECTED'}), {key:'',action:''});
});

test('installation progress has no retry action and confirmed failures retain theirs', () => {
  assert.deepEqual(inputSetupNotice({inputSetup:'FAILED'},true), {key:'input_setup_UPDATING',action:''});
  for (const inputSetup of ['UPDATING','PENDING']) {
    assert.deepEqual(inputSetupNotice({inputSetup}), {key:'input_setup_' + inputSetup,action:''});
  }
  for (const inputSetup of ['NOT_INSTALLED','UPDATE_REQUIRED','CANCELLED','FAILED']) {
    assert.deepEqual(inputSetupNotice({inputSetup}), {key:'input_setup_' + inputSetup,action:inputSetup === 'NOT_INSTALLED' ? 'input_setup' : 'input_refresh'});
  }
});
