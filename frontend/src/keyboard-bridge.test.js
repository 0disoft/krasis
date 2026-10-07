import test from 'node:test';
import assert from 'node:assert/strict';
import { createKeyboardBridge, emptyKeyboard, keyboardMatchBlock } from './keyboard-bridge.js';
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
test('matching explains unavailable peer state and requires both fresh lock observations', () => {
  const value = {...emptyKeyboard(peer),code:'ok',local:{caps:'OFF',num:'OFF',hangul:'UNKNOWN'},remote:{caps:'ON',num:'OFF',hangul:'UNKNOWN'}};
  assert.equal(keyboardMatchBlock(value,peer),'');
  assert.equal(keyboardMatchBlock({...value,remote:{...value.remote,caps:'UNKNOWN'}},peer),'remote_unknown');
  assert.equal(keyboardMatchBlock({...value,local:{...value.local,num:'UNKNOWN'}},peer),'local_unknown');
  assert.equal(keyboardMatchBlock(value,'b'.repeat(64)),'waiting');
  assert.equal(keyboardMatchBlock({...value,code:'unsupported'},peer),'unsupported');
  assert.equal(keyboardMatchBlock(value,peer,true),'busy');
});
test('foreign or malformed keyboard states never enable matching as known', async () => {
  for (const value of [{...emptyKeyboard(peer),peer:'b'.repeat(64)}, {...emptyKeyboard(peer),remote:{caps:'yes',num:'OFF',hangul:'UNKNOWN'}}]) {
    const bridge = createKeyboardBridge({KeyboardRefresh:async()=>value});
    assert.equal((await bridge.refresh(peer)).remote.caps,'UNKNOWN');
  }
});
