const schema = 'krasis.app-settings/v1';
const codes = new Set(['ok','unavailable','startup_failed','save_failed']);
export const emptyAppSettings = () => ({schema,code:'unavailable',startAtLogin:false,autoUpdate:false});
export function createAppSettingsBridge(binding) {
  let busy = false;
  async function call(method, ...args) {
    if (busy) return emptyAppSettings();
    busy = true;
    try {
      const value = await binding[method](...args);
      if (!value || value.schema !== schema || !codes.has(value.code) || typeof value.startAtLogin !== 'boolean' || typeof value.autoUpdate !== 'boolean') return emptyAppSettings();
      return {schema,code:value.code,startAtLogin:value.startAtLogin,autoUpdate:value.autoUpdate};
    } catch { return emptyAppSettings(); }
    finally { busy = false; }
  }
  const releaseStates = new Set(['IDLE','CHECKING','UNCONFIGURED','FAILED','CURRENT','AVAILABLE','DOWNLOADING','READY']);
  async function release(method) {
    try {
      const value=await binding[method]();
      if(!value || !releaseStates.has(value.state) || !Number.isInteger(value.generation) || value.generation<0 || value.generation>1000000) return {state:'FAILED',generation:0};
      return {state:value.state,generation:value.generation};
    } catch { return {state:'FAILED',generation:0}; }
  }
  return {status:()=>call('AppSettings'),set:(startup,update)=>typeof startup==='boolean'&&typeof update==='boolean'?call('SetAppSettings',startup,update):Promise.resolve(emptyAppSettings()),release:()=>release('ReleaseStatus'),check:()=>release('CheckRelease')};
}
