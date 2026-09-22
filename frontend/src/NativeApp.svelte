<script>
  import { onMount, tick } from 'svelte';
  import UpdateSettings from './UpdateSettings.svelte';
  import { createUpdateSettingsBridge } from './update-settings-bridge.js';
  import Icon from './Icon.svelte';
  import logo from '../../assets/windows/krasis.ico';
  import monitor from './assets/monitor.png';
  import { createBridge } from './bridge.js';
  import { createClipboardBridge } from './clipboard-bridge.js';
  import { createPairingBridge } from './pairing-bridge.js';
  import { createLayoutBridge } from './layout-bridge.js';
  import { createDiagnosticsBridge } from './diagnostics-bridge.js';
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
  let clipboardBridge;
  let pairingBridge;
  let layoutBridge;
  let diagnosticsBridge = $state.raw();
  let diagnostics = $state({code:'unavailable',saved:false,received:false,transfer:'UNKNOWN'});
  let diagBusy = $state(false);
  let diagAction = $state(false);
  let diagNotice = $state('');
  let diagnosticsDlg;
  let updateSettingsBridge = $state.raw();
  let updatesDlg;
  let updatesOpen = $state(false);
  async function openUpdates() { updatesOpen = true; await tick(); updatesDlg.showModal(); }
  let clipboard = $state({peer:'',code:'unavailable'});
  let pairing = $state({schema:'krasis.pairing/v1',code:'ok',pin:'',job:'',alias:'',candidates:[]});
  let layout = $state({edge:'RIGHT',rearm:32,code:'unavailable'});
  let candidate = $state('');
  let pinInput = $state('');
  let clipBusy = $state(false);
  let pairBusy = $state(false);
  let pairNotice = $state('');
  let clipNotice = $state('');
  let consent;
  let pairingDlg;
  let consentPeer = '';
  let disposed = false;
  const t = key => nativeText(locale, key);
  const selectedPeer = $derived(snapshot.peers.find(p => p.fingerprint === selected));
  const canDisconnect = $derived(['CONNECTING','CONNECTED','RECONNECTING'].includes(snapshot.state));
  const canPlace = $derived(snapshot.state === 'LOCAL' && !!selectedPeer && !pairing.job && !busy);
  const canPair = $derived(snapshot.state === 'LOCAL' && !pairing.job && !busy && !pairBusy);
  async function runDiagnostics(action, peer = '') {
    if (!diagnosticsBridge || diagBusy || disposed) return;
    diagBusy = true;
    diagAction = action !== 'status';
    try {
      const value = await (action === 'status' ? diagnosticsBridge.status() :
        action === 'open' ? diagnosticsBridge.open() : diagnosticsBridge.export(peer));
      if (disposed) return;
      diagnostics = value;
      if (action !== 'status') diagNotice = action === 'export' && value.code === 'ok' ? 'diag_saved' : 'diag_' + value.code;
    } finally { if (!disposed) { diagBusy = false; diagAction = false; } }
  }
  async function setClipboard(peer,enable) {
    if (!clipboardBridge || clipBusy || disposed) return;
    clipBusy=true;
    try { const value=await clipboardBridge.set(peer,enable); if(!disposed) {clipboard=value;clipNotice=value.code==='ok'?'':value.code;} }
    finally {if(!disposed)clipBusy=false;}
  }
  function toggleClipboard() {
    if(clipboard.peer) {void setClipboard(clipboard.peer,false);return;}
    if(!selectedPeer)return;
    consentPeer=selectedPeer.fingerprint;
    consent.showModal();
  }
  async function runPair(action) {
    if (!pairingBridge || pairBusy || disposed) return;
    pairBusy = true;
    if (action !== 'status') pairNotice = '';
    try {
      const value = action === 'invite' ? await pairingBridge.invite()
        : action === 'search' ? await pairingBridge.search()
        : action === 'cancel' ? await pairingBridge.cancel()
        : action === 'join' ? await pairingBridge.join(candidate, pinInput)
        : await pairingBridge.status();
      if (disposed) return;
      applyPair(value);
      if (value.candidates[0] && !value.candidates.some(c => c.id === candidate)) candidate = value.candidates[0].id;
      if (action === 'join') pinInput = '';
      if (value.code === 'pairing_completed' || value.code === 'paired_connect_failed') await run('refresh');
    } finally { if (!disposed) pairBusy = false; }
  }
  function applyPair(value) {
    if (value.code === 'busy') return;
    if (value.code === 'unavailable' && pairing.job && !value.job) {
      value = {...value, job: pairing.job};
    }
    pairing = value;
    if (['pairing_completed', 'paired_connect_failed', 'pairing_ended'].includes(value.code)) {
      pairNotice = value.code;
    }
  }
  async function swap() {
    if (!layoutBridge || !selectedPeer || !canPlace) return;
    layout = await layoutBridge.set(selectedPeer.fingerprint, layout.edge === 'LEFT' ? 'RIGHT' : 'LEFT');
  }
  async function setRearm(distance) {
    if (!layoutBridge || !selectedPeer || !canPlace) return;
    layout = await layoutBridge.rearm(selectedPeer.fingerprint, distance);
  }
  async function run(action) {
    if (busy || !bridge || disposed) return;
    busy = true;
    try {
      const value = await (action === 'connect' ? bridge.connect(selectedPeer) : action === 'disconnect' ? bridge.disconnect() : action === 'restart' ? bridge.restart() : bridge.refresh());
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
        await Call.ByName('github.com/0disoft/krasis/internal/workspacehost.WorkspaceSmoke.Report', snapshot.schema ?? '', snapshot.state, snapshot.code);
        return;
      }
      if (!clipBusy && !disposed && clipboardBridge) {
        clipBusy=true;
        const value=await clipboardBridge.status();
        if(!disposed) {clipboard=value;clipBusy=false;}
      }
      if (!pairBusy && !disposed && pairingBridge) {
        await runPair('status');
      }
      if (!disposed && layoutBridge && selected) {
        const value = await layoutBridge.status(selected);
        if (!disposed) layout = value;
      }
      if (!disposed && !diagBusy && diagnosticsBridge) await runDiagnostics('status');
      if (!disposed) timer = setTimeout(poll, 1500);
    }
    void (async () => {
      try {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        if (disposed) return;
        const name = 'github.com/0disoft/krasis/internal/workspacehost.WorkspaceService.';
        bridge = createBridge({
          Refresh: () => Call.ByName(name + 'Refresh'),
          Connect: (alias, fingerprint) => Call.ByName(name + 'Connect', alias, fingerprint),
          Disconnect: () => Call.ByName(name + 'Disconnect'),
          Restart: () => Call.ByName(name + 'Restart'),
        });
        clipboardBridge=createClipboardBridge({
          ClipboardStatus:()=>Call.ByName(name+'ClipboardStatus'),
          ClipboardSet:(peer,enable)=>Call.ByName(name+'ClipboardSet',peer,enable),
        });
        pairingBridge=createPairingBridge({
          PairStatus:()=>Call.ByName(name+'PairStatus'),
          PairInvite:()=>Call.ByName(name+'PairInvite'),
          PairSearch:()=>Call.ByName(name+'PairSearch'),
          PairJoin:(id,pin)=>Call.ByName(name+'PairJoin',id,pin),
          PairCancel:()=>Call.ByName(name+'PairCancel'),
        });
        layoutBridge=createLayoutBridge({
          LayoutStatus:(fp)=>Call.ByName(name+'LayoutStatus',fp),
          LayoutSet:(fp,edge)=>Call.ByName(name+'LayoutSet',fp,edge),
          LayoutRearm:(fp,distance)=>Call.ByName(name+'LayoutRearm',fp,distance),
        });
        diagnosticsBridge=createDiagnosticsBridge({
          DiagnosticsStatus:()=>Call.ByName(name+'DiagnosticsStatus'),
          DiagnosticsExport:peer=>Call.ByName(name+'DiagnosticsExport',peer),
          DiagnosticsOpen:()=>Call.ByName(name+'DiagnosticsOpen'),
        });
        updateSettingsBridge=createUpdateSettingsBridge({
          SendUpdate:peer=>Call.ByName(name+'SendUpdate',peer),
          UpdateTransfer:peer=>Call.ByName(name+'UpdateTransfer',peer),
          UpdateSettings:peer=>Call.ByName(name+'UpdateSettings',peer),
          UpdatePermission:(peer,allow)=>Call.ByName(name+'UpdatePermission',peer,allow),
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
      <button disabled={busy || !selectedPeer || !!pairing.job} onclick={() => run('connect')}>{t('connect')}</button>
    {:else if snapshot.state === 'FAULTED' || snapshot.state === 'UNKNOWN'}
      <button disabled={busy || !bridge} onclick={() => run('restart')}>{t('restart')}</button>
    {:else}
      <button disabled={busy || !canDisconnect} onclick={() => run('disconnect')}>{t('disconnect')}</button>
    {/if}
  </div>
  <section class="heading"><h1>{t('devices')}</h1><p>{t('help')}</p></section>
  <p class="fault-banner" role="status">{error ? t(error) : t('scope')}</p>
  <label class="peer-select">{t('peer')}<select bind:value={selected} disabled={busy || snapshot.state !== 'LOCAL'}><option value="">{t('choose')}</option>{#each snapshot.peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}</select></label>
  <section class="canvas" aria-label={t('devices')}>
    <div class="screens" class:reversed={layout.edge === 'LEFT'}>
      <div class="screen"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong>{t('local')}</strong><span>{t('thisPC')}</span><small>{t(layout.edge === 'LEFT' ? 'right' : 'left')}</small></div></div>
      <div class="screen peer"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong class="peer-name" dir="auto">{snapshot.peerAlias || selectedPeer?.alias || '—'}</strong><span>{t('otherPC')}</span><small>{t(layout.edge === 'LEFT' ? 'left' : 'right')}</small></div></div>
    </div>
    <p class="placement-hint"><Icon name="info" size={20} />{t(snapshot.state === 'LOCAL' ? 'placementReady' : 'placementLocked')}</p>
    <div class="canvas-actions">
      <button disabled={!canPlace} onclick={swap}><Icon name="arrows-left-right" size={18} />{t('swap')}</button>
      <select disabled={!canPlace} value={String(layout.rearm || 32)} onchange={e => setRearm(Number(e.currentTarget.value))} aria-label={t('rearm')}>
        {#each [32,64,128] as value}<option {value}>{t('rearm')} {value}px</option>{/each}
      </select>
      <button onclick={() => pairingDlg.showModal()}><Icon name="plus" size={18} />{t('addDevice')}</button>
    </div>
  </section>
  <section class="utilities">
    <button class="utility utility-button" disabled={clipBusy || (!clipboard.peer && !selectedPeer)} onclick={toggleClipboard}><Icon name="clipboard" /><div><h2>{t('clipboard')}</h2><p>{t(clipboard.peer ? 'clipboard_on' : 'clipboard_off')}</p></div></button>
    <button class="utility utility-button" onclick={openUpdates}><Icon name="arrows-clockwise" /><div><h2>{t('update')}</h2><p>{t('upd_title')}</p></div></button>
    <button class="utility utility-button" onclick={() => pairingDlg.showModal()}><Icon name="plus" /><div><h2>{t('pair')}</h2><p>{t(pairing.job ? 'pin_waiting' : 'pairHelp')}</p></div></button>
    <button class="utility utility-button" onclick={() => diagnosticsDlg.showModal()}><Icon name="info" /><div><h2>{t('diag_title')}</h2><p>{t('diag_save')} · {t('diag_open')}</p></div></button>
  </section>
  <p role="status">{clipNotice ? t(clipNotice) : clipboard.code !== 'ok' ? t(clipboard.code) : layout.code !== 'ok' && layout.code !== 'unavailable' && layout.code !== 'unconfigured' ? t(layout.code) : t('clipboard_help')}</p>
</main>
<dialog bind:this={updatesDlg} aria-labelledby="update-settings-title" onclose={() => updatesOpen = false}>
  {#if updatesOpen}<UpdateSettings bridge={updateSettingsBridge} peers={snapshot.peers} initialPeer={selected} {locale} />{/if}
  <button class="primary" onclick={() => updatesDlg.close()}>{t('close')}</button>
</dialog>
<dialog bind:this={diagnosticsDlg} aria-labelledby="diagnostics-title">
  <h2 id="diagnostics-title">{t('diag_title')}</h2>
  <p>{t('diag_help')}</p>
  <p>{t('diag_local')}: {t(diagnostics.saved ? 'diag_available' : diagnostics.code === 'unavailable' ? 'diag_unavailable' : 'diag_none')}</p>
  <p>{t('diag_received')}: {t(diagnostics.received ? 'diag_available' : diagnostics.code === 'unavailable' ? 'diag_unavailable' : 'diag_none')}</p>
  <p>{t('diag_latest')}: {t('diag_transfer_' + diagnostics.transfer)}</p>
  <p>{t('diag_global')}</p>
  <div class="pair-actions">
    <button disabled={diagBusy || !diagnosticsBridge} onclick={() => runDiagnostics('export')}>{t('diag_save')}</button>
    <button disabled={diagBusy || !diagnosticsBridge || !selectedPeer} onclick={() => runDiagnostics('export', selectedPeer.fingerprint)}>{t('diag_send')}{selectedPeer ? ' · ' + selectedPeer.alias : ''}</button>
    <button disabled={diagBusy || !diagnosticsBridge} onclick={() => runDiagnostics('open')}>{t('diag_open')}</button>
  </div>
  <p role="status">{diagAction ? t('diag_busy') : diagNotice ? t(diagNotice) : diagnostics.code !== 'ok' ? t('diag_' + diagnostics.code) : ''}</p>
  <button class="primary" onclick={() => diagnosticsDlg.close()}>{t('close')}</button>
</dialog>
<dialog bind:this={consent} aria-labelledby="clipboard-consent-title">
  <h2 id="clipboard-consent-title">{t('clipboard')}</h2>
  <p>{t('clipboard_consent')}</p>
  <button onclick={()=>consent.close()}>{t('cancel')}</button>
  <button onclick={()=>{consent.close();void setClipboard(consentPeer,true);}}>{t('clipboard_enable')}</button>
</dialog>
<dialog bind:this={pairingDlg} aria-labelledby="pair-title">
  <h2 id="pair-title">{t('pairingTitle')}</h2>
  <p>{t('pairingHelp')}</p>
  {#if pairing.pin}<p class="pin-display">{pairing.pin}</p>{/if}
  <div class="pair-actions">
    <button disabled={!canPair} onclick={() => runPair('invite')}>{t('createPIN')}</button>
    <button disabled={!canPair} onclick={() => runPair('search')}>{t('findDevice')}</button>
    <button disabled={pairBusy || !pairing.job} onclick={() => runPair('cancel')}>{t('cancelPair')}</button>
  </div>
  {#if pairing.candidates.length}
    <label class="peer-select">{t('nearby')}<select bind:value={candidate} disabled={!canPair}>{#each pairing.candidates as item, index (item.id)}<option value={item.id}>{t('nearbyDevice')} {index + 1}</option>{/each}</select></label>
    <label class="peer-select">{t('enterPIN')}<input bind:value={pinInput} maxlength="6" inputmode="numeric" autocomplete="one-time-code" disabled={!canPair} /></label>
    <button disabled={!canPair || !candidate || !/^\d{6}$/.test(pinInput)} onclick={() => runPair('join')}>{t('joinPIN')}</button>
  {/if}
  <p role="status">{t(pairNotice || pairing.code)}</p>
  <button class="primary" onclick={() => pairingDlg.close()}>{t('close')}</button>
</dialog>
<footer><span>{t('closeHelp')}</span><select aria-label={t('language')} bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></footer>
<style>
  .peer-select { display:flex; align-items:center; gap:20px; margin:20px 0; }
  .peer-select select, .peer-select input { min-width:220px; }
  .canvas { min-height:280px; height:35vh; }
  .screen { max-width:340px; }
  .screen-content strong { font-size:30px; }
  .peer-name { max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .fault-banner { font-size:14px; }
  .brand { white-space:nowrap; }
  .pin-display { font: 600 42px/1 ui-monospace, Consolas, monospace; letter-spacing: 12px; text-align:center; margin: 12px 0 4px; }
</style>
