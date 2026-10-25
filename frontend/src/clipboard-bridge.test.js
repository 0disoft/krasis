import test from 'node:test';
import assert from 'node:assert/strict';
import {createClipboardBridge,projectClipboardTransfer,clipboardTransferLabelKey} from './clipboard-bridge.js';
import {nativeCatalogs} from './native-i18n.js';

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
 assert.deepEqual(projectClipboardTransfer({state:'FAILED',format:'files',code:'size_limit',bytes:0,total:0,path:'private',text:'private'}),{format:'files',state:'FAILED',code:'size_limit',bytes:0,total:0});
 assert.equal(projectClipboardTransfer({state:'SENDING',format:'files',bytes:100,total:10}),null);
 assert.equal(projectClipboardTransfer({state:'FAILED',format:'text'}),null);
 assert.equal(projectClipboardTransfer({state:'PRIVATE',format:'files'}),null);
 assert.equal(projectClipboardTransfer({state:'FAILED',format:'files',code:'private path'}).code,'');
});

test('text and image failures reach the UI with format-specific limits and no content',()=>{
 for (const format of ['', 'png', 'dib']) {
  const value=projectClipboardTransfer({format,state:'FAILED',code:'size_limit',text:'private',path:'private',id:'private',error:'private'});
  assert.deepEqual(Object.keys(value).sort(),['bytes','code','format','state','total']);
  assert.equal(clipboardTransferLabelKey(value),format===''?'clipboard_text_size_limit':'clipboard_image_size_limit');
  const limit=format===''?64*1024:40*1024*1024;
  assert.equal(projectClipboardTransfer({format,state:'FAILED',bytes:limit+1}),null);
  assert.equal(projectClipboardTransfer({format,state:'READY'}),null);
  for (const code of ['prepare_failed','apply_failed','receive_failed','expired','apply_expired','private path']) {
   const failure=projectClipboardTransfer({format,state:'FAILED',code});
   const key=clipboardTransferLabelKey(failure);
   assert.ok(!key.includes('private'));
   for (const locale of ['ko','en']) assert.ok(nativeCatalogs[locale][key],`${locale} missing ${key}`);
  }
 }
 assert.equal(projectClipboardTransfer({state:'FAILED',code:'apply_failed'}).format,'');
 assert.equal(projectClipboardTransfer({format:null,state:'FAILED'}),null);
});
test('clipboard failures do not expose native exceptions or guess success',async()=>{
 const b=createClipboardBridge({ClipboardStatus:async()=>{throw new Error('private path')}});
 assert.deepEqual(await b.status(),{peer:'',code:'unavailable'});
});
