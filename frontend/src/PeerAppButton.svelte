<script>
  import { onDestroy } from 'svelte';
  import { nativeText } from './native-i18n.js';
  import { appOpenProgress } from './app-open-bridge.js';
  import Icon from './Icon.svelte';
  let { bridge, peer, locale, compact = false } = $props();
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
      if(pending){busy=true;result=pending;follow(pending);}
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
    if (!['pending','launch_accepted','launch_requested'].includes(next.code)) { busy = false; return; }
    follow(next);
  }
  function follow(next){
    const fingerprint=next.peer;
    const deadline=performance.now()+Math.min(70000,next.remaining??70000);
    async function poll() {
      if (disposed) return;
      const status = await bridge.status(fingerprint, next.id);
      if (disposed) return;
      const progress = appOpenProgress(result, status, bridge.expired ? bridge.expired(fingerprint,next.id) : performance.now() >= deadline);
      result = progress.result;
      if (progress.waiting) { timer = setTimeout(poll,1500); return; }
      busy = false;
    }
    timer = setTimeout(poll,1500);
  }
  onDestroy(() => { disposed = true; clearTimeout(timer); });
</script>

<div class="peer-app-action" class:compact>
  <button type="button" disabled={!bridge || !peer || busy} aria-label={t('app_open') + (peer ? ' · ' + peer.alias : '')} title={t('app_open') + (peer ? ' · ' + peer.alias : '')} onclick={open}>
    {#if compact}<Icon name="monitor" size={18} />{:else}{t('app_open')}{peer ? ' · ' + peer.alias : ''}{/if}
  </button>
  {#if visible && (!compact || visible.code !== 'window_ready')}<span role="status">{t('app_open_' + visible.code)}</span>{/if}
</div>
<style>
  .peer-app-action { display:flex; flex-wrap:wrap; align-items:center; gap:12px; margin:12px 0; }
  span { color:var(--muted, #b5bac8); }
  .compact { margin:0; gap:8px; font-size:12px; min-width:0; }
  .compact button { width:36px; height:36px; padding:0; flex-shrink:0; }
  .compact span { flex-basis:100%; max-width:240px; overflow-wrap:anywhere; }
</style>
