import test from 'node:test';
import assert from 'node:assert/strict';
import {createClipboardBridge,projectClipboardTransfer} from './clipboard-bridge.js';

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

test('transfer metadata cannot expose paths or raw reasons',()=>{
 assert.deepEqual(projectClipboardTransfer({state:'FAILED',format:'files',code:'size_limit',bytes:0,total:0,path:'private',text:'private'}),{state:'FAILED',code:'size_limit',bytes:0,total:0});
 assert.equal(projectClipboardTransfer({state:'SENDING',format:'files',bytes:100,total:10}),null);
 assert.equal(projectClipboardTransfer({state:'FAILED',format:'text'}),null);
 assert.equal(projectClipboardTransfer({state:'PRIVATE',format:'files'}),null);
 assert.equal(projectClipboardTransfer({state:'FAILED',format:'files',code:'private path'}).code,'');
});
test('clipboard failures do not expose native exceptions or guess success',async()=>{
 const b=createClipboardBridge({ClipboardStatus:async()=>{throw new Error('private path')}});
 assert.deepEqual(await b.status(),{peer:'',code:'unavailable'});
});
