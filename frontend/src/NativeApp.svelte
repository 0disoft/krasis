<script>
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import logo from '../../assets/windows/krasis.ico';
  import monitor from './assets/monitor.png';
  import { createBridge } from './bridge.js';
  import { resolveLocale } from './i18n.js';
  import { nativeText } from './native-i18n.js';
  let saved;
  try { saved = localStorage.getItem('krasis.workspace.locale'); } catch {}
  let locale = $state(resolveLocale(saved, navigator.languages));
  let snapshot = $state({state:'UNKNOWN', peerAlias:'', peers:[], code:'unavailable'});
  let selected = $state('');
  let busy = $state(false);
  let error = $state('');
  let bridge = $state.raw();
  let disposed = false;
  const t = key => nativeText(locale, key);
  const selectedPeer = $derived(snapshot.peers.find(p => p.fingerprint === selected));
  const canDisconnect = $derived(['CONNECTING','CONNECTED','RECONNECTING'].includes(snapshot.state));
  async function run(action) {
    if (busy || !bridge || disposed) return;
    busy = true;
    try {
      const value = await (action === 'connect' ? bridge.connect(selectedPeer) : action === 'disconnect' ? bridge.disconnect() : bridge.refresh());
      if (disposed) return;
      snapshot = value;
      if (!value.peers.some(p => p.fingerprint === selected)) {
        selected = (value.peers.find(p => p.alias === value.peerAlias) ?? value.peers[0])?.fingerprint ?? '';
      }
      error = value.code === 'ok' ? '' : value.code;
    } catch {
      if (!disposed) { snapshot = {state:'UNKNOWN', peerAlias:'', peers:[], code:'unavailable'}; error = 'unavailable'; }
    } finally { if (!disposed) busy = false; }
  }
  onMount(() => {
    let timer;
    async function poll() {
      await run('refresh');
      if (new URLSearchParams(location.search).get('smoke') === '1') {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        await Call.ByName('main.WorkspaceSmoke.Report', snapshot.schema ?? '', snapshot.state, snapshot.code);
        return;
      }
      if (!disposed) timer = setTimeout(poll, 1500);
    }
    void (async () => {
      try {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        if (disposed) return;
        const name = 'github.com/0disoft/krasis/internal/productpanel.DesktopBridge.';
        bridge = createBridge({
          Refresh: () => Call.ByName(name + 'Refresh'),
          Connect: (alias, fingerprint) => Call.ByName(name + 'Connect', alias, fingerprint),
          Disconnect: () => Call.ByName(name + 'Disconnect'),
        });
        await poll();
      } catch { if (!disposed) error = 'unavailable'; }
    })();
    return () => { disposed = true; clearTimeout(timer); };
  });
  $effect(() => {
    document.documentElement.lang = locale;
    try { localStorage.setItem('krasis.workspace.locale', locale); } catch {}
  });
</script>

<header><span class="brand"><img src={logo} alt="" />Krasis</span><nav aria-label="Krasis"><button class="active" aria-current="page"><Icon name="monitor" />{t('workspace')}</button></nav><span class="preview-label">{t('integration')}</span></header>
<main>
  <div class="connection-bar">
    <span class="connection" class:fault={snapshot.state !== 'CONNECTED'}><span class="status-dot"></span>{t(snapshot.state)}</span>
    <button disabled={busy || !bridge} onclick={() => run('refresh')}>{t('refresh')}</button>
    {#if snapshot.state === 'LOCAL'}
      <button disabled={busy || !selectedPeer} onclick={() => run('connect')}>{t('connect')}</button>
    {:else}
      <button disabled={busy || !canDisconnect} onclick={() => run('disconnect')}>{t('disconnect')}</button>
    {/if}
  </div>
  <section class="heading"><h1>{t('devices')}</h1><p>{t('help')}</p></section>
  <p class="fault-banner" role="status">{error ? t(error) : t('scope')}</p>
  <label class="peer-select">{t('peer')}<select bind:value={selected} disabled={busy || snapshot.state !== 'LOCAL'}><option value="">{t('choose')}</option>{#each snapshot.peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}</select></label>
  <section class="canvas" aria-label={t('devices')}>
    <div class="screens">
      <div class="screen"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong>{t('local')}</strong><span>{t('thisPC')}</span></div></div>
      <div class="screen peer"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong class="peer-name" dir="auto">{snapshot.peerAlias || selectedPeer?.alias || '—'}</strong><span>{t('otherPC')}</span></div></div>
    </div>
    <p class="placement-hint">{t('placement')}</p>
  </section>
  <section class="utilities">
    <button class="utility utility-button" disabled><Icon name="clipboard" /><div><h2>{t('clipboard')}</h2><p>{t('legacy')}</p></div></button>
    <button class="utility utility-button" disabled><Icon name="arrows-clockwise" /><div><h2>{t('update')}</h2><p>{t('legacy')}</p></div></button>
    <button class="utility utility-button" disabled><Icon name="plus" /><div><h2>{t('pair')}</h2><p>{t('legacy')}</p></div></button>
  </section>
</main>
<footer><span>{t('closeHelp')}</span><select aria-label={t('language')} bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></footer>
<style>
  .peer-select { display:flex; align-items:center; gap:20px; margin:20px 0; }
  .peer-select select { min-width:220px; }
  .canvas { min-height:280px; height:35vh; }
  .screen { max-width:340px; }
  .screen-content strong { font-size:30px; }
  .peer-name { max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .fault-banner { font-size:14px; }
  .brand { white-space:nowrap; }
</style>
