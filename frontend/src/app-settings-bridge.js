const schema = 'krasis.app-settings/v1';
const codes = new Set(['ok','unavailable','startup_failed','save_failed','start_menu_failed','recovery_failed','rollback_failed']);
export const emptyAppSettings = () => ({schema,code:'unavailable',startAtLogin:false,autoUpdate:false,preferencesKnown:false});
const releaseNotices = {
  ko: {IDLE:'',CHECKING:'업데이트 확인 중',UNCONFIGURED:'공개 배포 준비 중',FAILED:'업데이트 확인 실패',CANCELLED:'자동 업데이트 중지됨',CURRENT:'최신 버전',AVAILABLE:'새 버전 있음',DOWNLOADING:'업데이트 다운로드 중'},
  en: {IDLE:'',CHECKING:'Checking for updates',UNCONFIGURED:'Public releases not configured',FAILED:'Update check failed',CANCELLED:'Automatic updating stopped',CURRENT:'Up to date',AVAILABLE:'Update available',DOWNLOADING:'Downloading update'}
};
export function appReleaseNotice(state, settings, locale = 'ko', readyGeneration = 0) {
  const ko = locale === 'ko';
  if (state !== 'READY' && readyGeneration > 0) {
    const retained = ko ? '다운로드한 업데이트 보관 중' : 'Downloaded update retained';
    return `${releaseNotices[ko ? 'ko' : 'en'][state] ?? ''} · ${retained}`;
  }
  if (state !== 'READY') return releaseNotices[ko ? 'ko' : 'en'][state] ?? '';
  if (!settings || !settings.preferencesKnown || settings.code === 'unavailable' || typeof settings.autoUpdate !== 'boolean') {
    return ko ? '다운로드 완료 · 자동 업데이트 설정 확인 필요' : 'Downloaded · Verify automatic update settings';
  }
  if (!settings.autoUpdate) {
    return ko ? '다운로드 완료 · 자동 업데이트를 켜면 적용' : 'Downloaded · Enable automatic updates to apply';
  }
  return ko ? '연결 해제 후 자동 적용' : 'Will apply after disconnecting';
}
export function createAppSettingsBridge(binding) {
  let busy = false;
  async function call(method, ...args) {
    if (busy) return emptyAppSettings();
    busy = true;
    try {
      const value = await binding[method](...args);
      if (!value || value.schema !== schema || !codes.has(value.code) || typeof value.startAtLogin !== 'boolean' || typeof value.autoUpdate !== 'boolean') return emptyAppSettings();
      if (value.preferencesKnown !== undefined && typeof value.preferencesKnown !== 'boolean') return emptyAppSettings();
      // Older hosts only prove loaded preferences with an ok response.
      const preferencesKnown = value.preferencesKnown ?? (value.code === 'ok');
      if (value.code === 'ok' && !preferencesKnown) return emptyAppSettings();
      return {schema,code:value.code,startAtLogin:preferencesKnown && value.startAtLogin,autoUpdate:preferencesKnown && value.autoUpdate,preferencesKnown};
    } catch { return emptyAppSettings(); }
    finally { busy = false; }
  }
  const releaseStates = new Set(['IDLE','CHECKING','UNCONFIGURED','FAILED','CANCELLED','CURRENT','AVAILABLE','DOWNLOADING','READY']);
  async function release(method) {
    try {
      const value=await binding[method]();
      if(!value || !releaseStates.has(value.state) || !Number.isInteger(value.generation) || value.generation<0 || value.generation>1000000) return {state:'FAILED',generation:0};
      const readyGeneration = value.readyGeneration ?? 0;
      if (!Number.isInteger(readyGeneration) || readyGeneration < 0 || readyGeneration > 1000000) return {state:'FAILED',generation:0,readyGeneration:0};
      return {state:value.state,generation:value.generation,readyGeneration};
    } catch { return {state:'FAILED',generation:0}; }
  }
  async function set(startup,update) {
    if(typeof startup!=='boolean'||typeof update!=='boolean') return emptyAppSettings();
    const result=await call('SetAppSettings',startup,update);
    if(result.code==='ok') return result;
    const actual=await call('AppSettings');
    return {...actual,code:result.code};
  }
  return {status:()=>call('AppSettings'),set,release:()=>release('ReleaseStatus'),check:()=>release('CheckRelease')};
}
