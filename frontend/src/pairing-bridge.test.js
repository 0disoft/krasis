import test from 'node:test';
import assert from 'node:assert/strict';
import { createPairingBridge } from './pairing-bridge.js';

test('pairing adapter keeps addresses and native errors out of the UI', async () => {
  const id = 'a'.repeat(32);
  let received;
  const b = createPairingBridge({
    PairStatus: async () => ({schema: 'krasis.pairing/v1', code: 'ok', pin: '', job: '', candidates: [{id, address: '192.168.1.9:45871'}]}),
    PairInvite: async () => ({schema: 'krasis.pairing/v1', code: 'pin_waiting', pin: '123456', job: id, token: 'secret'}),
    PairSearch: async () => { throw new Error('udp 192.168.1.9'); },
    PairJoin: async (...args) => { received = args; return {schema: 'krasis.pairing/v1', code: 'pairing_completed', pin: '999999', job: '', alias: 'PC-aaaaaaaa'}; },
    PairCancel: async () => ({schema: 'krasis.pairing/v1', code: 'pairing_ended'}),
  });
  assert.deepEqual(await b.status(), {schema: 'krasis.pairing/v1', code: 'ok', pin: '', job: '', alias: '', candidates: [{id}]});
  assert.equal((await b.invite()).pin, '123456');
  assert.equal((await b.search()).code, 'unavailable');
  assert.equal((await b.join('nope', '123456')).code, 'invalid_pin');
  const joined = await b.join(id, '654321');
  assert.deepEqual(received, [id, '654321']);
  assert.equal(joined.pin, '');
  assert.equal(joined.alias, 'PC-aaaaaaaa');
});
