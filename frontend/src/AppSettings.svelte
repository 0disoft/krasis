<script>
  import { onMount } from 'svelte';
  import { createAppSettingsBridge, emptyAppSettings } from './app-settings-bridge.js';
  let { locale = 'ko' } = $props();
  let value = $state(emptyAppSettings());
  let busy = $state(true);
  let bridge;
  let disposed = false;
  const ko = $derived(locale === 'ko');
  async function change(startup, update) {
    if (!bridge || busy || disposed) return;
    busy = true;
    const next = await bridge.set(startup,update);
    if (!disposed) { value=next; busy=false; }
  }
  onMount(() => {
    void (async () => {
      try {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        const name='github.com/0disoft/krasis/internal/workspacehost.WorkspaceService.';
        bridge=createAppSettingsBridge({AppSettings:()=>Call.ByName(name+'AppSettings'),SetAppSettings:(startup,update)=>Call.ByName(name+'SetAppSettings',startup,update)});
        const next=await bridge.status();
        if(!disposed)value=next;
      } finally { if(!disposed)busy=false; }
    })();
    return()=>{disposed=true;};
  });
</script>
<div class="app-settings">
  <label><input type="checkbox" checked={value.startAtLogin} disabled={busy || value.code === 'unavailable'} onchange={e=>change(e.currentTarget.checked,value.autoUpdate)} />{ko?'Windows 시작 시 실행':'Start with Windows'}</label>
  <label><input type="checkbox" checked={value.autoUpdate} disabled={busy || value.code === 'unavailable'} onchange={e=>change(value.startAtLogin,e.currentTarget.checked)} />{ko?'자동 업데이트':'Automatic updates'}</label>
  {#if value.code !== 'ok'}<span role="status">{ko?'설정을 확인하지 못했습니다.':'Could not verify settings.'}</span>{/if}
</div>
<style>.app-settings{display:flex;flex-wrap:wrap;gap:16px}.app-settings label{display:flex;align-items:center;gap:8px;font-size:14px}.app-settings input{accent-color:var(--purple,#8751cd);width:16px;height:16px}.app-settings span{font-size:13px}</style>
