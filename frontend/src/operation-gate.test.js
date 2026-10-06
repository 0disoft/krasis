import test from 'node:test';
import assert from 'node:assert/strict';
import { createOperationGate } from './operation-gate.js';
import { createBridge } from './bridge.js';

const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};

test('status polling leaves controls enabled and skips overlapping polls', async () => {
  const visible = [];
  const run = createOperationGate(value => visible.push(value));
  const query = deferred();
  const pending = run(() => query.promise, true);
  assert.equal(await run(() => assert.fail('duplicate poll'), true), undefined);
  assert.deepEqual(visible, []);
  query.resolve('CONNECTED');
  assert.equal(await pending, 'CONNECTED');
  assert.deepEqual(visible, []);
});

test('a click during polling waits and calls the native bridge exactly once', async () => {
  const visible = [], calls = [];
  const query = deferred();
  const value = state => ({ schema:'krasis.desktop/v1', state, peerAlias:'B', peers:[], code:'ok', notice:'' });
  const bridge = createBridge({
    Refresh: () => { calls.push('query'); return query.promise; },
    Connect() {},
    Disconnect: async () => { calls.push('disconnect'); return value('LOCAL'); },
  });
  const run = createOperationGate(value => visible.push(value));
  const poll = run(() => bridge.refresh(), true);
  const click = run(() => bridge.disconnect());
  assert.deepEqual(visible, [true]);
  assert.deepEqual(calls, ['query']);
  await run(() => assert.fail('poll overtook a waiting click'), true);
  await run(() => assert.fail('duplicate click'));
  query.resolve(value('CONNECTED'));
  await poll;
  assert.equal((await click).state, 'LOCAL');
  assert.deepEqual(calls, ['query', 'disconnect']);
  assert.deepEqual(visible, [true, false]);
});

test('failed poll releases the gate without discarding a queued action', async () => {
  const visible = [];
  const run = createOperationGate(value => visible.push(value));
  const query = deferred();
  const poll = run(() => query.promise, true);
  const failure = assert.rejects(poll, /offline/);
  const click = run(async () => 'sent');
  query.reject(Error('offline'));
  await failure;
  assert.equal(await click, 'sent');
  assert.deepEqual(visible, [true, false]);
  assert.equal(await run(async () => 'refreshed', true), 'refreshed');
});

test('failed action restores controls and allows the next query', async () => {
  const visible = [];
  const run = createOperationGate(value => visible.push(value));
  await assert.rejects(run(() => { throw Error('failed'); }), /failed/);
  assert.deepEqual(visible, [true, false]);
  assert.equal(await run(async () => 'current', true), 'current');
  assert.deepEqual(visible, [true, false]);
});
