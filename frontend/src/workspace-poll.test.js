import test from 'node:test';
import assert from 'node:assert/strict';
import { createWorkspacePoll, pollStatus } from './workspace-poll.js';
import { createOperationGate } from './operation-gate.js';
import { createBridge } from './bridge.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return {promise, resolve, reject};
}
const snapshot = (state, revision) => ({schema:'krasis.desktop/v1', state, revision, peerAlias:'', peers:[], code:'ok', notice:''});

test('disconnect starts during a delayed workspace read and its result stays visible', async () => {
  const query = deferred(), calls = [], applied = [];
  let busy = false;
  const bridge = createBridge({
    Refresh: () => { calls.push('refresh'); return query.promise; },
    Connect() {},
    Disconnect: async () => { calls.push('disconnect'); return snapshot('LOCAL', 8); },
  });
  const poll = createWorkspacePoll({read:bridge.refresh, ready:()=>!busy, apply:v=>applied.push(v), failed:()=>assert.fail('stale poll failure')});
  const operation = createOperationGate(value => { busy = value; if (value) poll.invalidate(); });
  const reading = poll.refresh();
  await poll.refresh(); // No overlapping cached read.
  assert.equal(busy, false);
  await operation(async () => applied.push(await bridge.disconnect()));
  assert.deepEqual(calls, ['refresh','disconnect']);
  query.resolve(snapshot('CONNECTED', 7));
  await reading;
  assert.deepEqual(applied.map(v=>v.state), ['LOCAL']);
  assert.equal(busy, false);
});

test('an old refresh failure cannot erase an action result and the next read recovers', async () => {
  const query = deferred(), applied = [];
  let failed = 0, next = query.promise;
  const poll = createWorkspacePoll({read:()=>next, ready:()=>true, apply:v=>applied.push(v), failed:()=>failed++});
  const reading = poll.refresh();
  poll.invalidate();
  query.reject(Error('late IPC failure'));
  await reading;
  assert.equal(failed, 0);
  next = Promise.reject(Error('current IPC failure'));
  await poll.refresh();
  assert.equal(failed, 1);
  next = Promise.resolve(snapshot('CONNECTED', 9));
  await poll.refresh();
  assert.equal(applied[0].revision, 9);
});

test('a read cannot update a disposed or busy workspace', async () => {
  const query = deferred();
  let ready = true;
  const poll = createWorkspacePoll({read:()=>query.promise, ready:()=>ready,
    apply:()=>assert.fail('applied while blocked'), failed:()=>assert.fail('reported while blocked')});
  const reading = poll.refresh();
  ready = false;
  query.resolve(snapshot('LOCAL', 3));
  await reading;
});

test('independent status lanes start together and settle despite one failed lane', async () => {
  const slow = deferred(), calls = [];
  const cycle = pollStatus([
    async () => { calls.push('workspace'); await slow.promise; calls.push('layout'); },
    async () => { calls.push('clipboard'); throw Error('offline'); },
    async () => { calls.push('pairing'); },
    async () => { calls.push('diagnostics'); },
    async () => { calls.push('replacement'); },
  ]);
  assert.deepEqual(calls, ['workspace','clipboard','pairing','diagnostics','replacement']);
  slow.resolve();
  const results = await cycle;
  assert.equal(calls.at(-1), 'layout');
  assert.deepEqual(results.map(v=>v.status), ['fulfilled','rejected','fulfilled','fulfilled','fulfilled']);
});
