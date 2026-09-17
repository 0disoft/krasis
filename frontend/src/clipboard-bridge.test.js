import test from 'node:test';
import assert from 'node:assert/strict';
import {createClipboardBridge} from './clipboard-bridge.js';

test('clipboard adapter keeps only acknowledged settings, never content',async()=>{
 const peer='a'.repeat(64);let received;
 const b=createClipboardBridge({
  ClipboardStatus:async()=>({peer,code:'ok',text:'private clipboard'}),
  ClipboardSet:async(...args)=>{received=args;return {peer:'',code:'ok'}},
 });
 assert.deepEqual(await b.status(),{peer,code:'ok'});
 assert.deepEqual(await b.set(peer,false),{peer:'',code:'ok'});
 assert.deepEqual(received,[peer,false]);
 assert.equal((await b.set('../secret',true)).code,'invalid_peer');
});
test('clipboard failures do not expose native exceptions or guess success',async()=>{
 const b=createClipboardBridge({ClipboardStatus:async()=>{throw new Error('private path')}});
 assert.deepEqual(await b.status(),{peer:'',code:'unavailable'});
});
