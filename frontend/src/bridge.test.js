import test from 'node:test';
import assert from 'node:assert/strict';
import { createBridge } from './bridge.js';

const peer = { alias: 'B', fingerprint: 'a'.repeat(64) };
const snapshot = state => ({ schema: 'krasis.desktop/v1', state, peerAlias: '', peers: [peer], code: 'ok', notice: '' });
test('native adapter preserves authoritative connecting state and narrows output', async () => {
  const calls = [];
  const bridge = createBridge({ Refresh: async () => ({ ...snapshot('LOCAL'), token: 'not-rendered' }),
    Connect: async (...args) => { calls.push(args); return snapshot('CONNECTING'); },
    Disconnect: async () => snapshot('LOCAL') });
  assert.equal((await bridge.refresh()).token, undefined);
  assert.equal((await bridge.connect(peer)).state, 'CONNECTING');
  assert.deepEqual(calls, [[peer.alias, peer.fingerprint]]);
  assert.equal((await bridge.disconnect()).state, 'LOCAL');
});
test('missing binding, malformed snapshot and native failures never become mock success', async () => {
  assert.throws(() => createBridge(undefined), /bridge_unavailable/);
  const binding = { Refresh: async () => ({ state: 'CONNECTED' }), Connect() {}, Disconnect() {} };
  await assert.rejects(createBridge(binding).refresh(), /bridge_protocol/);
  binding.Refresh = async () => { throw new Error('private token'); };
  await assert.rejects(createBridge(binding).refresh(), { message: 'bridge_failed' });
});
test('overlapping calls are rejected and failed calls release the gate', async () => {
  let finish;
  const binding = { Refresh: () => new Promise(resolve => { finish = resolve; }),
    Connect: async () => snapshot('CONNECTING'), Disconnect: async () => snapshot('LOCAL') };
  const bridge = createBridge(binding);
  const pending = bridge.refresh();
  await assert.rejects(bridge.disconnect(), /bridge_busy/);
  finish(snapshot('LOCAL')); await pending;
  await assert.rejects(bridge.connect({ alias: 'B' }), /bridge_invalid_peer/);
  assert.equal((await bridge.disconnect()).state, 'LOCAL');
});
