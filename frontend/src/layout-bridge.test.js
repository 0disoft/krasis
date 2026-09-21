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
