import test from 'node:test';
import assert from 'node:assert/strict';
import { createLayoutBridge } from './layout-bridge.js';

test('layout adapter never forwards addresses and rejects unknown edges', async () => {
  const id = 'a'.repeat(64);
  let received;
  const b = createLayoutBridge({
    LayoutStatus: async () => ({edge: 'LEFT', rearm: 64, code: 'ok', address: '192.168.1.9:45872'}),
    LayoutSet: async (...args) => { received = args; return {edge: 'RIGHT', rearm: 32, code: 'ok'}; },
    LayoutRearm: async () => ({edge: 'LEFT', rearm: 128, code: 'ok'}),
  });
  assert.deepEqual(await b.status(id), {edge: 'LEFT', rearm: 64, code: 'ok'});
  assert.equal((await b.set('nope', 'LEFT')).code, 'invalid_peer');
  assert.deepEqual(await b.set(id, 'RIGHT'), {edge: 'RIGHT', rearm: 32, code: 'ok'});
  assert.deepEqual(received, [id, 'RIGHT']);
});

test('malformed successful layouts never become default settings', async () => {
  const id = 'a'.repeat(64);
  for (const value of [null, {}, {code:'ok',edge:'UP',rearm:32}, {code:'ok',edge:'LEFT',rearm:999}, {code:'ok',edge:'LEFT'}, {code:'private error',edge:'LEFT',rearm:32}]) {
    const binding = {LayoutStatus:async()=>value, LayoutSet:async()=>value, LayoutRearm:async()=>value};
    const bridge = createLayoutBridge(binding);
    assert.deepEqual(await bridge.status(id), {code:'unavailable'});
    assert.deepEqual(await bridge.set(id,'LEFT'), {code:'unavailable'});
    assert.deepEqual(await bridge.rearm(id,64), {code:'unavailable'});
  }
  const bridge = createLayoutBridge({LayoutStatus:async()=>({code:'unconfigured'}), LayoutSet:async()=>{throw Error('private');}, LayoutRearm:async()=>({code:'rejected',edge:'RIGHT',rearm:32})});
  assert.deepEqual(await bridge.status(id),{edge:'RIGHT',rearm:32,code:'unconfigured'});
  assert.deepEqual(await bridge.set(id,'LEFT'),{code:'unavailable'});
  assert.deepEqual(await bridge.rearm(id,64),{code:'rejected'});
});
