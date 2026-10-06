import test from 'node:test';
import assert from 'node:assert/strict';
import { createLayoutBridge } from './layout-bridge.js';
import { createLayoutController } from './layout-controller.js';
import { createOperationGate } from './operation-gate.js';

const peer = 'a'.repeat(64), other = 'b'.repeat(64);
const value = edge => ({ edge, rearm:32, code:'ok' });
function fixture() {
  let selected = peer, local = true, finish;
  const applied = [], calls = [], busy = [];
  const bridge = createLayoutBridge({
    LayoutStatus: () => { calls.push('read'); return new Promise(resolve => { finish = resolve; }); },
    LayoutSet: async (id, edge) => { calls.push(['set',id,edge]); return value(edge); },
    LayoutRearm: async (id, distance) => { calls.push(['rearm',id,distance]); return {...value('RIGHT'),rearm:distance}; },
  });
  const controller = createLayoutController(bridge, createOperationGate(v => busy.push(v)), {
    currentPeer: () => selected, canChange: () => local, apply: v => applied.push(v),
  });
  return { controller, applied, calls, busy,
    select: id => { selected=id; }, connect: () => { local=false; }, finish: v => finish(v) };
}

test('slow layout poll completes before a queued edit and cannot undo it', async () => {
  const f = fixture();
  const read = f.controller.status(peer);
  assert.deepEqual(f.busy, []);
  const edit = f.controller.set(peer,'LEFT');
  assert.deepEqual(f.calls,['read']);
  f.finish(value('RIGHT'));
  await read; await edit;
  assert.deepEqual(f.calls,['read',['set',peer,'LEFT']]);
  assert.deepEqual(f.applied,[value('RIGHT'),value('LEFT')]);
  assert.deepEqual(f.busy,[true,false]);
});

test('changing the selected peer discards the old poll and queued edit', async () => {
  const f = fixture();
  const read = f.controller.status(peer);
  const edit = f.controller.rearm(peer,128);
  f.select(other);
  f.finish(value('LEFT'));
  await read; await edit;
  assert.deepEqual(f.applied,[]);
  assert.deepEqual(f.calls,['read']);
});

test('a queued layout edit rechecks whether the agent connected meanwhile', async () => {
  const f = fixture();
  const read = f.controller.status(peer);
  const edit = f.controller.set(peer,'LEFT');
  f.connect(); f.finish(value('RIGHT'));
  await read; await edit;
  assert.deepEqual(f.calls,['read']);
  assert.deepEqual(f.applied,[value('RIGHT')]);
});
