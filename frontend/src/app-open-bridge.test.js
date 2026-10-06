import test from 'node:test';
import assert from 'node:assert/strict';
import { createAppOpenBridge, appOpenProgress } from './app-open-bridge.js';
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
test('reload restores the same request and clears it only after window confirmation',async()=>{
  const values=new Map();const storage={getItem:k=>values.get(k),setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)};
  const first=createAppOpenBridge({PeerAppOpen:async()=>value},storage);
  await first.open(peer);
  let writes=0;
  const next=createAppOpenBridge({PeerAppOpen:async(p,r)=>{assert.equal(r,id);writes++;return {...value,code:'window_ready'};}},storage);
  assert.equal(next.resume(peer).id,id);
  assert.equal(next.resume('c'.repeat(64)),undefined);
  assert.equal((await next.status(peer,id)).code,'window_ready');
  assert.equal(writes,1);
  assert.equal(next.resume(peer),undefined);
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

test('launch acknowledgement becomes visible while awaiting window readiness',()=>{
  const launch={...value,code:'launch_requested'};
  assert.deepEqual(appOpenProgress(value,launch,false),{result:launch,waiting:true});
  assert.deepEqual(appOpenProgress(launch,{...value,code:'unavailable'},false),{result:launch,waiting:true});
  assert.equal(appOpenProgress(launch,{...value,code:'query_busy'},true).result.code,'window_unconfirmed');
  const rejected={...value,code:'busy'};
  assert.deepEqual(appOpenProgress(value,rejected,false),{result:rejected,waiting:false});
  const failed={...value,code:'peer_unresponsive'};
  assert.deepEqual(appOpenProgress(value,failed,false),{result:failed,waiting:false});
});

test('no-response receipt clears the saved request without sending a new launch',async()=>{
  const values=new Map();const storage={getItem:k=>values.get(k),setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)};
  const calls=[];
  const bridge=createAppOpenBridge({PeerAppOpen:async(p,r)=>{
    calls.push([p,r]);return {...value,code:r?'peer_unresponsive':'pending'};
  }},storage);
  await bridge.open(peer);
  assert.equal((await bridge.status(peer,id)).code,'peer_unresponsive');
  assert.equal(bridge.resume(peer),undefined);
  assert.deepEqual(calls,[[peer,''],[peer,id]]);
});
