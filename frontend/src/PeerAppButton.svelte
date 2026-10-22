<script>
  import { nativeText } from './native-i18n.js';
  import { createAppOpenController } from './app-open-controller.js';
  import Icon from './Icon.svelte';
  let { bridge, peer, locale, compact = false } = $props();
  let busy = $state(false);
  let result = $state();
  let controller;
  const t = key => nativeText(locale, key);
  const visible = $derived(result?.peer === peer?.fingerprint ? result : undefined);
  $effect(()=>{
    const fingerprint=peer?.fingerprint;
    const binding=bridge;
    busy=false;result=undefined;
    if(!binding||!fingerprint)return;
    const next=createAppOpenController(binding,{apply:value=>{busy=value.busy;result=value.result;}});
    controller=next;
    next.resume(fingerprint);
    return ()=>{next.dispose();if(controller===next)controller=undefined;};
  });
  function open() { controller?.open(peer?.fingerprint); }
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
