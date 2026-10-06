import test from 'node:test';
import assert from 'node:assert/strict';
import { createKeyboardBridge, emptyKeyboard } from './keyboard-bridge.js';
const peer = 'a'.repeat(64);

test('keyboard observations strip content and matching is one explicit local request', async () => {
  let calls = 0;
  const value = {...emptyKeyboard(peer),code:'ok',remote:{caps:'ON',num:'OFF',hangul:'UNKNOWN'},path:'/private',pin:'secret'};
  const bridge = createKeyboardBridge({KeyboardRefresh:async()=>value,KeyboardMatch:async p=>{calls++;assert.equal(p,peer);return {...value,code:'sent'};}});
  const snapshot = await bridge.refresh(peer);
  assert.equal(snapshot.path,undefined);
  assert.equal(calls,0);
  assert.equal((await bridge.match(peer)).code,'sent');
  assert.equal(calls,1);
});
test('foreign or malformed keyboard states never enable matching as known', async () => {
  for (const value of [{...emptyKeyboard(peer),peer:'b'.repeat(64)}, {...emptyKeyboard(peer),remote:{caps:'yes',num:'OFF',hangul:'UNKNOWN'}}]) {
    const bridge = createKeyboardBridge({KeyboardRefresh:async()=>value});
    assert.equal((await bridge.refresh(peer)).remote.caps,'UNKNOWN');
  }
});
