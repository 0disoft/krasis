<script>
  import { onDestroy } from 'svelte';
  import { nativeText } from './native-i18n.js';
  let { bridge, peer, locale } = $props();
  let busy = $state(false);
  let result = $state();
  let disposed = false;
  let timer;
  const t = key => nativeText(locale, key);
  const visible = $derived(result?.peer === peer?.fingerprint ? result : undefined);
  $effect(()=>{
    const fingerprint=peer?.fingerprint;
    if(bridge&&fingerprint&&!busy&&!result){
      const pending=bridge.resume?.(fingerprint);
      if(pending){busy=true;result=pending;follow(pending,pending.expires);}
    }
  });
  async function open() {
    if (busy || !bridge || !peer || disposed) return;
    busy = true;
    const fingerprint = peer.fingerprint;
    result = {peer:fingerprint, code:'pending'};
    const next = await bridge.open(fingerprint);
    if (disposed) return;
    result = next;
    if (next.code !== 'pending') { busy = false; return; }
    follow(next,Date.now()+70000);
  }
  function follow(next,deadline){
    const fingerprint=next.peer;
    async function poll() {
      if (disposed) return;
      const status = await bridge.status(fingerprint, next.id);
      if (disposed) return;
      if (['pending','launch_requested','busy','unavailable'].includes(status.code)) {
        if (Date.now() < deadline) { timer = setTimeout(poll,1500); return; }
        result = {...next,code:'outcome_unknown'};
      } else result = status;
      busy = false;
    }
    timer = setTimeout(poll,1500);
  }
  onDestroy(() => { disposed = true; clearTimeout(timer); });
</script>

<div class="peer-app-action">
  <button type="button" disabled={!bridge || !peer || busy} onclick={open}>{t('app_open')}{peer ? ' · ' + peer.alias : ''}</button>
  {#if visible}<span role="status">{t('app_open_' + visible.code)}</span>{/if}
</div>
<style>
  .peer-app-action { display:flex; flex-wrap:wrap; align-items:center; gap:12px; margin:12px 0; }
  span { color:var(--muted, #b5bac8); }
</style>
