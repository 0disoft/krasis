import test from 'node:test';
import assert from 'node:assert/strict';
import { createAppOpenController } from './app-open-controller.js';
import { createAppOpenBridge } from './app-open-bridge.js';

const peer = 'a'.repeat(64), id = 'b'.repeat(32);
const receipt = (code = 'pending') => ({ schema: 'krasis.app-open/v1', peer, id, code });
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};
async function flush() { for (let i = 0; i < 8; i++) await Promise.resolve(); }
function fixture(bridge, extra = {}) {
  let now = 0, next = 0;
  const timers = new Map(), states = [];
  const scheduler = {
    setTimeout(fn, delay) { const id = ++next; timers.set(id, { at: now + delay, fn }); return id; },
    clearTimeout(id) { timers.delete(id); },
  };
  const controller = createAppOpenController(bridge, { clock: () => now, scheduler, apply: value => states.push(value), ...extra });
  return { controller, states, timers, clock: () => now,
    async advance(until) {
      await flush();
      while (true) {
        const due = [...timers].sort((a, b) => a[1].at - b[1].at).find(([, timer]) => timer.at <= until);
        if (!due) break;
        const [id, timer] = due;
        now = timer.at; timers.delete(id); timer.fn(); await flush();
      }
      now = until; await flush();
    },
  };
}

test('an unfinished status cannot extend observation beyond exactly 70 seconds', async () => {
  const query = deferred(); let launches = 0, reads = 0;
  const f = fixture({ open: async () => { launches++; return receipt(); }, status: () => { reads++; return query.promise; } });
  f.controller.open(peer);
  await f.advance(69999);
  assert.equal(f.states.at(-1).busy, true);
  assert.equal(reads, 1);
  await f.advance(70000);
  assert.deepEqual(f.states.at(-1), { busy: false, result: receipt('outcome_unknown') });
  assert.equal(f.timers.size, 0);
  const count = f.states.length;
  query.resolve(receipt('window_ready'));
  await f.advance(70001);
  assert.equal(f.states.length, count);
  assert.equal(launches, 1);
});

test('the deadline also bounds an initial launch with no reply and consumes late rejection', async () => {
  const launch = deferred(); let calls = 0;
  const f = fixture({ open: () => { calls++; return launch.promise; }, status: () => assert.fail('no request id') });
  assert.equal(f.controller.open(peer), true);
  assert.equal(f.controller.open(peer), false);
  await f.advance(70000);
  assert.equal(f.states.at(-1).busy, false);
  assert.equal(f.states.at(-1).result.code, 'outcome_unknown');
  const count = f.states.length;
  launch.reject(Error('late native failure'));
  await flush();
  assert.equal(f.states.length, count);
  assert.equal(calls, 1);
});

test('launch acknowledgement is preserved on timeout while transient queries do not renew time', async () => {
  let reads = 0;
  const f = fixture({ open: async () => receipt('pending'), status: async () => receipt(++reads === 1 ? 'launch_requested' : reads % 2 ? 'query_busy' : 'unavailable') });
  f.controller.open(peer);
  await f.advance(1500);
  assert.equal(f.states.at(-1).result.code, 'launch_requested');
  await f.advance(70000);
  assert.equal(f.states.at(-1).result.code, 'window_unconfirmed');
  assert.equal(f.states.at(-1).busy, false);
  assert.equal(f.timers.size, 0);
});

test('success just before the deadline wins, but a completion at the deadline cannot revive observation', async () => {
  for (const at of [69999, 70000, 70001]) {
    const query = deferred();
    const f = fixture({ open: async () => receipt(), status: () => query.promise });
    f.controller.open(peer); await f.advance(at);
    query.resolve(receipt('window_ready')); await flush();
    assert.equal(f.states.at(-1).result.code, at < 70000 ? 'window_ready' : 'outcome_unknown');
    assert.equal(f.timers.size, 0);
  }
});

test('resumed receipts query their original id without launching and keep the smaller remaining budget', async () => {
  const query = deferred(), calls = [];
  const f = fixture({ resume: () => ({ ...receipt(), remaining: 1000 }), open: () => assert.fail('replayed launch'),
    status: (p, r) => { calls.push([p, r]); return query.promise; } });
  assert.equal(f.controller.resume(peer), true);
  await f.advance(999);
  assert.equal(f.states.at(-1).busy, true);
  await f.advance(1000);
  assert.equal(f.states.at(-1).result.code, 'outcome_unknown');
  assert.deepEqual(calls, [[peer, id]]);
  f.controller.dispose(); query.resolve(receipt()); await flush();
});

test('reload of a locally expired receipt permits bounded native confirmation without issuing a launch', async () => {
  const f = fixture({ resume: () => ({ ...receipt(), remaining: 0 }), open: () => assert.fail('replayed launch'), status: async () => receipt('window_ready') });
  f.controller.resume(peer);
  await flush();
  assert.equal(f.states.at(-1).result.code, 'window_ready');
  assert.equal(f.states.at(-1).busy, false);
  assert.equal(f.timers.size, 0);
});

test('abort and dispose remove timers and ignore both late success and late failure', async () => {
  for (const abort of [false, true]) {
    const signal = new AbortController(), query = deferred();
    const f = fixture({ open: async () => receipt(), status: () => query.promise }, { signal: signal.signal });
    f.controller.open(peer); await f.advance(1500);
    if (abort) signal.abort(); else f.controller.dispose();
    const count = f.states.length;
    assert.equal(f.timers.size, 0);
    if (abort) assert.equal(f.states.at(-1).busy, false);
    assert.equal(f.controller.open(peer), false);
    if (abort) query.reject(Error('late failure')); else query.resolve(receipt('window_ready'));
    await flush();
    assert.equal(f.states.length, count);
  }
});

test('a pre-aborted signal performs no native work and synchronous adapter failure settles safely', async () => {
  const signal = new AbortController(); signal.abort();
  const f = fixture({ open: () => assert.fail('aborted launch') }, { signal: signal.signal });
  assert.equal(f.controller.open(peer), false);
  assert.equal(f.timers.size, 0);
  const broken = fixture({ open() { throw Error('native adapter unavailable'); } });
  broken.controller.open(peer); await flush();
  assert.equal(broken.states.at(-1).result.code, 'outcome_unknown');
  assert.equal(broken.timers.size, 0);
});

test('UI timeout cannot release the real bridge native-call fence or replay the original request', async () => {
  let f, launches = 0;
  const query = deferred();
  const bridge = createAppOpenBridge({ PeerAppOpen: async (p, r) => {
    if (!r) { launches++; return receipt(); }
    return query.promise;
  } }, { getItem: () => null, setItem() {}, removeItem() {} }, { wall: () => 1000, elapsed: () => f.clock() });
  f = fixture(bridge);
  f.controller.open(peer); await f.advance(70000);
  assert.equal(f.states.at(-1).busy, false);
  f.controller.open(peer); await flush();
  assert.equal(f.states.at(-1).result.code, 'query_busy');
  assert.equal(launches, 1);
  query.resolve(receipt('window_ready')); await flush();
  assert.equal(f.states.at(-1).result.code, 'query_busy');
  assert.equal(f.timers.size, 0);
});

test('callback mutation cannot change polling identity or the acknowledged timeout outcome', async () => {
  const calls = [], published = [];
  const f = fixture({
    open: async () => receipt(),
    status: async (p, r) => { calls.push([p, r]); return receipt('launch_requested'); },
  }, { apply: ({ busy, result }) => {
    published.push({ busy, result: { ...result } });
    result.id = 'changed'; result.code = 'pending';
  } });
  f.controller.open(peer);
  await f.advance(3000);
  assert.deepEqual(calls, [[peer, id], [peer, id]]);
  await f.advance(70000);
  assert.deepEqual(published.at(-1), { busy: false, result: receipt('window_unconfirmed') });
  assert.equal(f.timers.size, 0);
});

test('adapter-owned receipts cannot change retained identity or acknowledgement', async () => {
  for (const source of ['open', 'status', 'resume']) {
    const external = receipt('launch_requested'), calls = [];
    const f = fixture({
      open: async () => source === 'open' ? external : receipt(),
      resume: () => external,
      status: async (p, r) => {
        calls.push([p, r]);
        return source === 'status' && calls.length === 1 ? external : receipt('unavailable');
      },
    });
    if (source === 'resume') f.controller.resume(peer); else f.controller.open(peer);
    await f.advance(source === 'status' ? 1500 : 0);
    external.id = 'changed';
    external.code = 'pending';
    await f.advance(70000);
    assert.ok(calls.length > 0);
    assert.ok(calls.every(([p, r]) => p === peer && r === id), source);
    assert.equal(f.states.at(-1).result.code, 'window_unconfirmed', source);
    assert.equal(f.timers.size, 0);
  }
});

test('completion callbacks cannot mutate the adapter result', async () => {
  const external = receipt('window_ready');
  const f = fixture({ open: async () => external }, {
    apply: ({ busy, result }) => { if (!busy) result.id = 'changed'; },
  });
  f.controller.open(peer);
  await flush();
  assert.deepEqual(external, receipt('window_ready'));
  assert.equal(f.timers.size, 0);
});
