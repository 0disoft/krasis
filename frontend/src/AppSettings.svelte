<script>
  import { onMount } from 'svelte';
  import { createAppSettingsBridge, emptyAppSettings } from './app-settings-bridge.js';
  let { locale = 'ko', onStatus } = $props();
  let value = $state(emptyAppSettings());
  let busy = $state(true);
  let bridge = $state.raw();
  let disposed = false;
  let release = $state({state:'IDLE',generation:0});
  const notices = {ko:{IDLE:'',CHECKING:'업데이트 확인 중',UNCONFIGURED:'공개 배포 준비 중',FAILED:'업데이트 확인 실패',CURRENT:'최신 버전',AVAILABLE:'새 버전 있음',DOWNLOADING:'업데이트 다운로드 중',READY:'연결 해제 후 자동 적용'},en:{IDLE:'',CHECKING:'Checking for updates',UNCONFIGURED:'Public releases not configured',FAILED:'Update check failed',CURRENT:'Up to date',AVAILABLE:'Update available',DOWNLOADING:'Downloading update',READY:'Will apply after disconnecting'}};
  const ko = $derived(locale === 'ko');
  $effect(() => { if (!busy) onStatus?.(value.code, release.state); });
  async function change(startup, update) {
    if (!bridge || busy || disposed) return;
    busy = true;
    const next = await bridge.set(startup,update);
    if (!disposed) { value=next; busy=false; }
  }
  onMount(() => {
    let timer;
    async function pollRelease(){const next=await bridge.release();if(!disposed){release=next;timer=setTimeout(pollRelease,3000);}}
    void (async () => {
      try {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        const name='github.com/0disoft/krasis/internal/workspacehost.WorkspaceService.';
        bridge=createAppSettingsBridge({AppSettings:()=>Call.ByName(name+'AppSettings'),SetAppSettings:(startup,update)=>Call.ByName(name+'SetAppSettings',startup,update),ReleaseStatus:()=>Call.ByName(name+'ReleaseStatus'),CheckRelease:()=>Call.ByName(name+'CheckRelease')});
        const next=await bridge.status();
        if(!disposed)value=next;
        if(!disposed)void pollRelease();
      } catch { if(!disposed)value=emptyAppSettings();
      } finally { if(!disposed)busy=false; }
    })();
    return()=>{disposed=true;clearTimeout(timer);};
  });
</script>
<div class="app-settings">
  <label><input type="checkbox" checked={value.startAtLogin} disabled={busy || value.code === 'unavailable'} onchange={e=>change(e.currentTarget.checked,value.autoUpdate)} />{ko?'Windows 시작 시 실행':'Start with Windows'}</label>
  <label><input type="checkbox" checked={value.autoUpdate} disabled={busy || value.code === 'unavailable'} onchange={e=>change(value.startAtLogin,e.currentTarget.checked)} />{ko?'자동 업데이트':'Automatic updates'}</label>
  <button disabled={!bridge || ['CHECKING','DOWNLOADING'].includes(release.state)} onclick={async()=>{release=await bridge.check();}}>{ko?'업데이트 확인':'Check for updates'}</button>
  {#if release.state !== 'IDLE'}<span role="status">{notices[ko?'ko':'en'][release.state]}</span>{/if}
  {#if value.code === 'start_menu_failed'}<span role="status">{ko?'시작 메뉴 등록 실패':'Start menu registration failed'}</span>
  {:else if value.code === 'recovery_failed'}<span role="status">{ko?'앱 자동 복구 등록 실패':'App recovery registration failed'}</span>
  {:else if value.code !== 'ok'}<span role="status">{ko?'설정을 확인하지 못했습니다.':'Could not verify settings.'}</span>{/if}
</div>
<style>.app-settings{display:flex;flex-wrap:wrap;gap:16px}.app-settings label{display:flex;align-items:center;gap:8px;font-size:14px}.app-settings input{accent-color:var(--purple,#8751cd);width:16px;height:16px}.app-settings span{font-size:13px}</style>
