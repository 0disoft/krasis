import test from 'node:test';
import assert from 'node:assert/strict';
import { createAppSettingsBridge } from './app-settings-bridge.js';
test('failed and malformed settings do not advertise defaults as applied',async()=>{
  for(const value of [null,{schema:'krasis.app-settings/v1',code:'ok',startAtLogin:'true',autoUpdate:true}]) {
    const b=createAppSettingsBridge({AppSettings:()=>value});assert.equal((await b.status()).code,'unavailable');
  }
});
test('settings toggle is a single explicit native boolean write',async()=>{
  const calls=[];const b=createAppSettingsBridge({SetAppSettings:(...args)=>{calls.push(args);return {schema:'krasis.app-settings/v1',code:'ok',startAtLogin:false,autoUpdate:true}}});
  assert.equal((await b.set(false,true)).startAtLogin,false);assert.deepEqual(calls,[[false,true]]);
  await b.set('false',true);assert.equal(calls.length,1);
});
