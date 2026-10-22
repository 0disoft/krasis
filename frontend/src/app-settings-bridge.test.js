import test from 'node:test';
import assert from 'node:assert/strict';
import { appReleaseNotice, appSettingsNoticeCode, createAppSettingsBridge, emptyAppSettings } from './app-settings-bridge.js';
import contract from '../../internal/appsettings/status-contract.json' with { type: 'json' };

test('every shared settings outcome crosses the bridge without exposing native fields', async () => {
  for (const code of Object.values(contract.codes)) {
    const value = {schema:contract.schema,code,startAtLogin:true,autoUpdate:false,preferencesKnown:true};
    const bridge = createAppSettingsBridge({AppSettings:()=>({...value,privateError:'native detail'})});
    assert.deepEqual(await bridge.status(),value);
  }
  const bridge = createAppSettingsBridge({AppSettings:()=>({schema:contract.schema,code:'future_unknown',startAtLogin:true,autoUpdate:true,preferencesKnown:true})});
  assert.deepEqual(await bridge.status(),emptyAppSettings());
});

test('failed and malformed settings do not advertise defaults as applied',async()=>{
  for(const value of [null,{schema:'krasis.app-settings/v1',code:'ok',startAtLogin:'true',autoUpdate:true}]) {
    const b=createAppSettingsBridge({AppSettings:()=>value});assert.equal((await b.status()).code,'unavailable');
  }
});

test('pending settings reads do not become errors and completed failures stay visible',()=>{
  const initial=emptyAppSettings();
  assert.equal(appSettingsNoticeCode(initial,true),'checking');
  assert.equal(appSettingsNoticeCode(initial,false),'unavailable');
  assert.equal(appSettingsNoticeCode({...initial,code:'rollback_failed'},true),'checking');
  assert.equal(appSettingsNoticeCode({...initial,code:'rollback_failed'},false),'rollback_failed');
  assert.equal(appSettingsNoticeCode({...initial,code:'ok'},false),'ok');
});

test('registration query uncertainty preserves known preferences and release guidance',async()=>{
  for(const code of ['startup_query_failed','recovery_query_failed']) {
    const value={schema:'krasis.app-settings/v1',code,startAtLogin:true,autoUpdate:true,preferencesKnown:true};
    const b=createAppSettingsBridge({AppSettings:()=>({...value,privateError:'not for the renderer'})});
    const read=await b.status();
    assert.deepEqual(read,value);
    assert.equal(appSettingsNoticeCode(read,false),code);
    assert.equal(appReleaseNotice('READY',read,'en'),'Will apply after disconnecting');
    assert.equal(appReleaseNotice('READY',{...read,autoUpdate:false},'en'),'Downloaded · Enable automatic updates to apply');
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
  assert.deepEqual(await b.set(false,false),{schema:'krasis.app-settings/v1',code:'startup_failed',startAtLogin:true,autoUpdate:true,preferencesKnown:true});
});

test('unverified restoration remains explicit after reloading saved checks',async()=>{
  const b=createAppSettingsBridge({
    SetAppSettings:()=>({schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:false,autoUpdate:false}),
    AppSettings:()=>({schema:'krasis.app-settings/v1',code:'recovery_failed',startAtLogin:false,autoUpdate:true,preferencesKnown:true})
  });
  assert.deepEqual(await b.set(true,false),{schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:false,autoUpdate:true,preferencesKnown:true});
});

test('user cancellation is projected separately from a release failure',async()=>{
  const b=createAppSettingsBridge({ReleaseStatus:()=>({state:'CANCELLED',generation:219})});
  assert.deepEqual(await b.release(),{state:'CANCELLED',generation:219,readyGeneration:0});
});

test('cancellation and check failure retain a separately validated download',async()=>{
  for (const state of ['CANCELLED','FAILED','CHECKING']) {
    const b=createAppSettingsBridge({ReleaseStatus:()=>({state,generation:223,readyGeneration:222})});
    assert.deepEqual(await b.release(),{state,generation:223,readyGeneration:222});
    assert.match(appReleaseNotice(state,null,'ko',222),/보관 중/);
    assert.match(appReleaseNotice(state,null,'en',222),/retained/);
  }
  const b=createAppSettingsBridge({ReleaseStatus:()=>({state:'FAILED',generation:223,readyGeneration:'222'})});
  assert.deepEqual(await b.release(),{state:'FAILED',generation:0,readyGeneration:0});
});

test('downloaded release guidance follows confirmed automatic update preferences',()=>{
  const settings = {schema:'krasis.app-settings/v1',code:'ok',startAtLogin:true,autoUpdate:true,preferencesKnown:true};
  for (const locale of ['ko','en']) {
    const enabled = locale === 'ko' ? '연결 해제 후 자동 적용' : 'Will apply after disconnecting';
    const retained = locale === 'ko' ? '다운로드 완료 · 자동 업데이트를 켜면 적용' : 'Downloaded · Enable automatic updates to apply';
    const unknown = locale === 'ko' ? '다운로드 완료 · 자동 업데이트 설정 확인 필요' : 'Downloaded · Verify automatic update settings';
    assert.equal(appReleaseNotice('READY',settings,locale),enabled);
    assert.equal(appReleaseNotice('READY',{...settings,autoUpdate:false},locale),retained);
    assert.equal(appReleaseNotice('READY',settings,locale),enabled);
    for (const value of [null,emptyAppSettings(),{...settings,code:'unavailable'},{...settings,autoUpdate:'true'},{...settings,code:'rollback_failed',preferencesKnown:false}]) {
      assert.equal(appReleaseNotice('READY',value,locale),unknown);
    }
    // OS startup restoration failure does not invalidate the loaded update preference.
    assert.equal(appReleaseNotice('READY',{...settings,code:'rollback_failed',autoUpdate:false},locale),retained);
    assert.equal(appReleaseNotice('IDLE',settings,locale),'');
    assert.equal(appReleaseNotice('FAILED',settings,locale),locale === 'ko' ? '업데이트 확인 실패' : 'Update check failed');
  }
});

test('unknown preferences stay unknown separately from restoration failures',async()=>{
  for (const preferencesKnown of [false,undefined]) {
    const b=createAppSettingsBridge({AppSettings:()=>({schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:true,autoUpdate:true,preferencesKnown})});
    assert.deepEqual(await b.status(),{...emptyAppSettings(),code:'rollback_failed'});
  }
  for (const preferencesKnown of ['true',false]) {
    const b=createAppSettingsBridge({AppSettings:()=>({schema:'krasis.app-settings/v1',code:'ok',startAtLogin:true,autoUpdate:true,preferencesKnown})});
    assert.deepEqual(await b.status(),emptyAppSettings());
  }
});

test('failed-write reloading never falls back to values that are no longer readable',async()=>{
  const b=createAppSettingsBridge({
    SetAppSettings:()=>({schema:'krasis.app-settings/v1',code:'rollback_failed',startAtLogin:true,autoUpdate:true,preferencesKnown:true}),
    AppSettings:()=>({...emptyAppSettings(),code:'rollback_failed'})
  });
  assert.deepEqual(await b.set(false,false),{...emptyAppSettings(),code:'rollback_failed'});
});
