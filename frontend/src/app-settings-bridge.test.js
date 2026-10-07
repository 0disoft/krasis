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
test('failed write reloads persisted checks and retains the failure notice',async()=>{
  const b=createAppSettingsBridge({
    SetAppSettings:()=>({schema:'krasis.app-settings/v1',code:'startup_failed',startAtLogin:false,autoUpdate:false}),
    AppSettings:()=>({schema:'krasis.app-settings/v1',code:'ok',startAtLogin:true,autoUpdate:true})
  });
  assert.deepEqual(await b.set(false,false),{schema:'krasis.app-settings/v1',code:'startup_failed',startAtLogin:true,autoUpdate:true});
});

test('unverified restoration remains explicit after reloading saved checks',async()=>{
  const b=createAppSettingsBridge({
    SetAppSettings:()=>({schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:false,autoUpdate:false}),
    AppSettings:()=>({schema:'krasis.app-settings/v1',code:'recovery_failed',startAtLogin:false,autoUpdate:true})
  });
  assert.deepEqual(await b.set(true,false),{schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:false,autoUpdate:true});
});
