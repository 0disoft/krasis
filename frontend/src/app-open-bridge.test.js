import test from 'node:test';
import assert from 'node:assert/strict';
import { createAppOpenBridge } from './app-open-bridge.js';
const peer='a'.repeat(64), id='b'.repeat(32);
const value={schema:'krasis.app-open/v1',peer,id,code:'pending'};
test('peer launch sends once and polls the same receipt without exposing native fields',async()=>{
  const calls=[];
  const bridge=createAppOpenBridge({PeerAppOpen:async(p,r)=>{calls.push([p,r]);return {...value,code:r?'launch_requested':'pending',path:'private'};}});
  assert.deepEqual(await bridge.open(peer),value);
  assert.deepEqual(await bridge.status(peer,id),{...value,code:'launch_requested'});
  assert.deepEqual(calls,[[peer,''],[peer,id]]);
  assert.equal((await bridge.status(peer,'')).code,'invalid_peer');
  assert.equal((await bridge.open('../path')).code,'invalid_peer');
  assert.equal(calls.length,2);
});
test('lost replies are not retried and mismatched or premature success is rejected',async()=>{
  let calls=0;
  const failed=createAppOpenBridge({PeerAppOpen:async()=>{calls++;throw Error('private token');}});
  assert.equal((await failed.open(peer)).code,'outcome_unknown');
  assert.equal(calls,1);
  for(const v of [{...value,id:''},{...value,peer:'c'.repeat(64)},{...value,code:'launch_requested'},{...value,code:'private'}]){
    assert.equal((await createAppOpenBridge({PeerAppOpen:async()=>v}).open(peer)).code,'outcome_unknown');
  }
  assert.equal((await createAppOpenBridge({PeerAppOpen:async()=>({...value,id:'c'.repeat(32)})}).status(peer,id)).code,'unavailable');
});
