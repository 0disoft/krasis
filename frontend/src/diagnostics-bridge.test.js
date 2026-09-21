import test from 'node:test';
import assert from 'node:assert/strict';
import { createDiagnosticsBridge } from './diagnostics-bridge.js';

const value = { schema: 'krasis.diagnostics/v1', code: 'ok', saved: true, received: false, transfer: 'NONE' };
test('diagnostics adapter strips paths and preserves pending and partial success', async () => {
  const calls = [];
  const bridge = createDiagnosticsBridge({
    DiagnosticsStatus: async () => ({ ...value, received: true, transfer: 'SENT', path: 'private', token: 'private' }),
    DiagnosticsExport: async peer => { calls.push(peer); return { ...value, code: 'queued', transfer: 'QUEUED' }; },
    DiagnosticsOpen: async (...args) => { calls.push(args); return value; },
  });
  assert.deepEqual(await bridge.status(), { ...value, received: true, transfer: 'SENT' });
  assert.equal((await bridge.export('a'.repeat(64))).transfer, 'QUEUED');
  await bridge.open('ignored arbitrary path');
  assert.deepEqual(calls, ['a'.repeat(64), []]);
  const partial = createDiagnosticsBridge({ DiagnosticsExport: async () => ({ ...value, code: 'send_failed', transfer: 'UNKNOWN' }) });
  assert.deepEqual(await partial.export('a'.repeat(64)), { ...value, code: 'send_failed', transfer: 'UNKNOWN' });
});
test('diagnostics adapter rejects malformed identities, payloads and native errors', async () => {
  let calls = 0;
  const bridge = createDiagnosticsBridge({ DiagnosticsExport: async () => { calls++; return value; } });
  for (const peer of [undefined, null, '../private', 'a'.repeat(65), { path: 'private' }]) {
    assert.equal((await bridge.export(peer)).code, 'invalid_peer');
  }
  assert.equal(calls, 0);
  for (const malformed of [{ ...value, schema: 'wrong' }, { ...value, saved: 'yes' }, { ...value, code: 'secret' }, { ...value, transfer: 'secret' }]) {
    assert.equal((await createDiagnosticsBridge({ DiagnosticsStatus: async () => malformed }).status()).code, 'unavailable');
  }
  const bad = createDiagnosticsBridge({ DiagnosticsStatus: async () => { throw new Error('private token'); } });
  assert.equal(JSON.stringify(await bad.status()).includes('private'), false);
});
test('diagnostics adapter does not queue overlapping export behind a status request', async () => {
  let finish;
  let sends = 0;
  const bridge = createDiagnosticsBridge({ DiagnosticsStatus: () => new Promise(resolve => { finish = resolve; }),
    DiagnosticsExport: async () => { sends++; return value; } });
  const pending = bridge.status();
  assert.equal((await bridge.export('')).code, 'busy');
  assert.equal(sends, 0);
  finish(value); await pending;
  assert.equal((await bridge.export('')).code, 'ok');
  assert.equal(sends, 1);
});
