import test from 'node:test';
import assert from 'node:assert/strict';
import { createUpdateSettingsBridge, emptyUpdateSettings } from './update-settings-bridge.js';
const peer = 'a'.repeat(64);
const value = {...emptyUpdateSettings(peer),code:'ok',permission:'ALLOWED',remotePermission:'DENIED',peerState:'ONLINE',localGeneration:32,localBuild:'c'.repeat(40),peerGeneration:32,peerBuild:'c'.repeat(40),comparison:'SAME_BUILD'};
test('update settings separate permission directions and narrow native payloads', async () => {
  const calls = [];
  const bridge = createUpdateSettingsBridge({UpdateSettings:async()=>({...value,path:'private',token:'private'}),UpdatePermission:async(...args)=>{calls.push(args);return {...value,code:'permission_saved'};}});
  assert.deepEqual(await bridge.status(peer),value);
  assert.equal((await bridge.permit(peer,true)).permission,'ALLOWED');
  assert.deepEqual(calls,[[peer,true]]);
  assert.equal((await bridge.permit(peer,'ALLOW')).code,'invalid_peer');
  assert.equal((await bridge.permit('../path',true)).code,'invalid_peer');
  assert.equal(calls.length,1);
});
test('update settings reject cross-peer results and clear stale observations', async () => {
  for (const bad of [{...value,peer:'b'.repeat(64)},{...value,localBuild:'private path'},{...value,permission:true}]) {
    assert.equal((await createUpdateSettingsBridge({UpdateSettings:async()=>bad}).status(peer)).code,'unavailable');
  }
  const stale = await createUpdateSettingsBridge({UpdateSettings:async()=>({...value,peerState:'STALE'})}).status(peer);
  assert.equal(stale.peerBuild,'');assert.equal(stale.remotePermission,'UNKNOWN');assert.equal(stale.comparison,'UNKNOWN');
});
test('update settings never retry ambiguous permission writes or queue overlapping work', async () => {
  let finish, writes = 0;
  const bridge = createUpdateSettingsBridge({UpdateSettings:()=>new Promise(resolve=>{finish=resolve;}),UpdatePermission:async()=>{writes++;throw Error('private token');}});
  const pending = bridge.status(peer);
  assert.equal((await bridge.permit(peer,true)).code,'busy');assert.equal(writes,0);
  finish(value);await pending;
  const result = await bridge.permit(peer,true);
  assert.equal(result.code,'outcome_unknown');assert.equal(result.permission,'UNKNOWN');assert.equal(writes,1);
  assert.equal(JSON.stringify(result).includes('private'),false);
});
