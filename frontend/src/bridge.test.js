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

test('older native projections cannot replace newer state or mutate the accepted cache', async () => {
  const values = [{ ...snapshot('CONNECTED'), revision: 8 }, { ...snapshot('RECONNECTING'), revision: 7 },
    { ...snapshot('LOCAL'), revision: 9 }];
  const bridge = createBridge({ Refresh: async () => values.shift(), Connect() {}, Disconnect() {} });
  const accepted = await bridge.refresh();
  accepted.state = 'FAULTED'; accepted.peers[0].alias = 'mutated';
  const stale = await bridge.refresh();
  assert.equal(stale.state, 'CONNECTED'); assert.equal(stale.revision, 8); assert.equal(stale.peers[0].alias, 'B');
  assert.equal((await bridge.refresh()).revision, 9);
});

test('invalid revision is rejected and transport queries never invoke disconnect', async () => {
  let disconnects = 0;
  const bridge = createBridge({ Refresh: async () => ({ ...snapshot('CONNECTED'), revision: -1 }),
    Connect() {}, Disconnect() { disconnects++; } });
  await assert.rejects(bridge.refresh(), /bridge_protocol/);
  assert.equal(disconnects, 0);
});

test('input mode follows fallback and unknown values cannot imply PIN support', async () => {
  let mode = 'PROTECTED';
  const bridge = createBridge({ Refresh: async () => ({...snapshot('CONNECTED'), inputBackend: mode}), Connect() {}, Disconnect() {} });
  assert.equal((await bridge.refresh()).inputBackend, 'PROTECTED');
  mode = 'PORTABLE';
  assert.equal((await bridge.refresh()).inputBackend, 'PORTABLE');
  for (mode of [undefined, 'private path', {state:'PROTECTED'}]) {
    assert.equal((await bridge.refresh()).inputBackend, 'UNKNOWN');
  }
});
