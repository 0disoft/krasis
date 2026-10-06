<script>
  import { onMount, tick } from 'svelte';
  import UpdateSettings from './UpdateSettings.svelte';
  import AppSettings from './AppSettings.svelte';
  import PeerAppButton from './PeerAppButton.svelte';
  import { createAppOpenBridge } from './app-open-bridge.js';
  import { createUpdateSettingsBridge } from './update-settings-bridge.js';
  import Icon from './Icon.svelte';
  import logo from '../../assets/windows/krasis.ico';
  import monitor from './assets/monitor.png';
  import { createBridge } from './bridge.js';
  import { createClipboardBridge } from './clipboard-bridge.js';
  import { createPairingBridge } from './pairing-bridge.js';
  import { createLayoutBridge } from './layout-bridge.js';
  import { createLayoutController } from './layout-controller.js';
  import { createDiagnosticsBridge } from './diagnostics-bridge.js';
  import { resolveLocale } from './i18n.js';
  import { nativeText } from './native-i18n.js';
  import { createOperationGate } from './operation-gate.js';
  let saved;
  try { saved = localStorage.getItem('krasis.workspace.locale'); } catch {}
  let locale = $state(resolveLocale(saved, navigator.languages));
  let snapshot = $state({state:'UNKNOWN', peerAlias:'', peers:[], code:'unavailable', inputSetup:'UNKNOWN'});
  let selected = $state('');
  let busy = $state(false);
  let inputBusy = $state(false);
  let error = $state('');
  let bridge = $state.raw();
  let clipboardBridge;
  let pairingBridge;
  let layoutBridge;
  let layoutController;
  let diagnosticsBridge = $state.raw();
  let diagnostics = $state({code:'unavailable',saved:false,received:false,transfer:'UNKNOWN'});
  let diagBusy = $state(false);
  let diagAction = $state(false);
  let diagNotice = $state('');
  let diagnosticsDlg;
  let updateSettingsBridge = $state.raw();
  let appOpenBridge = $state.raw();
  let updatesDlg;
  let updatesOpen = $state(false);
  let opener = null;
  function focusFirstControl(dlg) { dlg?.querySelector('select:not([disabled]), input:not([disabled]), button:not([disabled])')?.focus(); }
  function openDialog(dlg) { if (!dlg || dlg.open) return; opener = document.activeElement; dlg.showModal(); focusFirstControl(dlg); }
  function restoreDialogFocus() { if (opener && opener.focus) opener.focus(); opener = null; }
  async function openUpdates() { if (!updatesDlg || updatesDlg.open) return; opener = document.activeElement; updatesOpen = true; await tick(); if (updatesDlg.open) { focusFirstControl(updatesDlg); return; } updatesDlg.showModal(); await tick(); focusFirstControl(updatesDlg); }
  let clipboard = $state({peer:'',code:'unavailable'});
  let pairing = $state({schema:'krasis.pairing/v1',code:'ok',pin:'',job:'',alias:'',candidates:[]});
  let layout = $state({edge:'RIGHT',rearm:32,code:'unavailable'});
  let layoutPeer = $state('');
  let candidate = $state('');
  let pinInput = $state('');
  let clipBusy = $state(false);
  let pairBusy = $state(false);
  let pairNotice = $state('');
  let clipNotice = $state('');
  let clipError = $state.raw();
  let consent;
  let pairingDlg;
  let consentPeer = '';
  let disposed = false;
  const workspaceOperation = createOperationGate(value => { if (!disposed) busy = value; });
  const clipboardOperation = createOperationGate(value => { if (!disposed) clipBusy = value; });
  const pairingOperation = createOperationGate(value => { if (!disposed) pairBusy = value; });
  const diagnosticsOperation = createOperationGate(value => { if (!disposed) diagBusy = value; });
  const t = key => nativeText(locale, key);
  const inputNotice = $derived(inputBusy ? 'UPDATING' : snapshot.inputSetup ?? 'UNKNOWN');
  const inputNeedsAction = $derived(['NOT_INSTALLED','UPDATE_REQUIRED','CANCELLED','FAILED'].includes(inputNotice));
  const inputNoticeKey = $derived(inputNotice === 'UNKNOWN' || inputNotice === 'READY' ? 'input_portable' : 'input_setup_' + inputNotice);
  const selectedPeer = $derived(snapshot.peers.find(p => p.fingerprint === selected));
  const clipboardKnown = $derived(clipboard.code === 'ok');
  const clipboardEnabled = $derived(clipboardKnown && !!clipboard.peer);
  const clipboardAlias = $derived(snapshot.peers.find(p => p.fingerprint === clipboard.peer)?.alias ?? '');
	const fileTransfer = $derived(clipboardEnabled ? clipboard.transfer : null);
	const fileTransferLabel = $derived(fileTransfer ? t('clipboard_files_' + (fileTransfer.state === 'FAILED' && fileTransfer.code ? fileTransfer.code : fileTransfer.state.toLowerCase())) : '');
  const canDisconnect = $derived(['CONNECTING','CONNECTED','RECONNECTING'].includes(snapshot.state));
  const shownEdge = $derived(layoutPeer && layoutPeer === selected ? layout.edge : null);
  const canPlace = $derived(snapshot.state === 'LOCAL' && !!selectedPeer && layoutPeer === selected && ['ok','unconfigured'].includes(layout.code) && !pairing.job && !busy);
  const canPair = $derived(snapshot.state === 'LOCAL' && !pairing.job && !busy && !pairBusy);
  async function runDiagnostics(action, peer = '', background = false) {
    if (!diagnosticsBridge || diagBusy || disposed) return;
    await diagnosticsOperation(async () => {
      if (disposed) return;
      diagAction = action !== 'status';
      try {
        const value = await (action === 'status' ? diagnosticsBridge.status() :
          action === 'open' ? diagnosticsBridge.open() : diagnosticsBridge.export(peer));
        if (disposed) return;
        diagnostics = value;
        if (action !== 'status') diagNotice = action === 'export' && value.code === 'ok' ? 'diag_saved' : 'diag_' + value.code;
      } finally { if (!disposed) diagAction = false; }
    }, background);
  }
  async function setClipboard(peer,enable) {
    if (!clipboardBridge || clipBusy || disposed) return;
    await clipboardOperation(async () => {
      if (disposed) return;
      const value=await clipboardBridge.set(peer,enable);
      if(!disposed) {clipboard=value;clipNotice=value.code==='ok'?'':value.code==='rejected'?'clipboard_rejected':value.code; if(clipNotice) { await tick(); clipError?.focus(); }}
    });
  }
  function toggleClipboard() {
    if (!clipboardKnown) return;
    if(clipboard.peer) {void setClipboard(clipboard.peer,false);return;}
    if(!selectedPeer)return;
    consentPeer=selectedPeer.fingerprint;
    openDialog(consent);
  }
  async function runPair(action, background = false) {
    if (!pairingBridge || pairBusy || disposed) return;
    const joinCandidate = candidate, joinPIN = pinInput;
    await pairingOperation(async () => {
      if (disposed) return;
      if (action !== 'status') pairNotice = '';
      const value = action === 'invite' ? await pairingBridge.invite()
        : action === 'search' ? await pairingBridge.search()
        : action === 'cancel' ? await pairingBridge.cancel()
        : action === 'join' ? await pairingBridge.join(joinCandidate, joinPIN)
        : await pairingBridge.status();
      if (disposed) return;
      applyPair(value);
      if (value.candidates[0] && !value.candidates.some(c => c.id === candidate)) candidate = value.candidates[0].id;
      if (action === 'join') pinInput = '';
      if (value.code === 'pairing_completed' || value.code === 'paired_connect_failed') await run('refresh', true);
    }, background);
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
    if (!layoutController || !selectedPeer || !canPlace) return;
    await layoutController.set(selectedPeer.fingerprint, layout.edge === 'LEFT' ? 'RIGHT' : 'LEFT');
  }
  async function setRearm(distance) {
    if (!layoutController || !selectedPeer || !canPlace) return;
    await layoutController.rearm(selectedPeer.fingerprint, distance);
  }
  async function run(action, background = false) {
    if (busy || !bridge || disposed) return;
    const peer = selectedPeer;
    await workspaceOperation(async () => {
      if (disposed) return;
      try {
        const value = await (action === 'input' ? bridge.inputRefresh() : action === 'connect' ? bridge.connect(peer) : action === 'disconnect' ? bridge.disconnect() : action === 'restart' ? bridge.restart() : bridge.refresh());
        if (disposed) return;
        snapshot = value;
        if (!value.peers.some(p => p.fingerprint === selected)) {
          selected = (value.peers.find(p => p.alias === value.peerAlias) ?? value.peers[0])?.fingerprint ?? '';
        }
        error = value.code === 'ok' || value.code.startsWith('input_refresh_') ? '' : value.code;
      } catch {
        if (!disposed) { snapshot = {state:'UNKNOWN', peerAlias:'', peers:[], code:'unavailable'}; error = 'unavailable'; }
      }
    }, background);
  }
  async function refreshInput() {
    if (busy || inputBusy || !bridge) return;
    inputBusy = true;
    try { await run('input'); }
    finally { if (!disposed) inputBusy = false; }
  }
  onMount(() => {
    let timer;
    async function poll() {
      await run('refresh', true);
      if (new URLSearchParams(location.search).get('smoke') === '1') {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        await Call.ByName('github.com/0disoft/krasis/internal/workspacehost.WorkspaceSmoke.Report', snapshot.schema ?? '', snapshot.state, snapshot.code);
        return;
      }
      if (!clipBusy && !disposed && clipboardBridge) {
        await clipboardOperation(async () => {
          if (disposed) return;
          const value=await clipboardBridge.status();
          if(!disposed) clipboard=value;
        }, true);
      }
      if (!pairBusy && !disposed && pairingBridge) {
        await runPair('status', true);
      }
      if (!disposed && layoutController && selected) await layoutController.status(selected);
      if (!disposed && !diagBusy && diagnosticsBridge) await runDiagnostics('status', '', true);
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
          InputRefresh: () => Call.ByName(name + 'InputRefresh'),
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
        layoutController=createLayoutController(layoutBridge, workspaceOperation, {
          currentPeer: () => disposed ? '' : selected,
          canChange: () => snapshot.state === 'LOCAL' && layoutPeer === selected && ['ok','unconfigured'].includes(layout.code) && !pairing.job && !inputBusy,
          apply: (value, peer) => { layout = value; layoutPeer = peer; },
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
        appOpenBridge=createAppOpenBridge({PeerAppOpen:(peer,id)=>Call.ByName(name+'PeerAppOpen',peer,id)});
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
    <span class="connection" class:fault={snapshot.state !== 'CONNECTED'} role="status" aria-atomic="true"><span class="status-dot"></span>{t(snapshot.state)}</span>
    <button disabled={busy || !bridge} onclick={() => run(snapshot.state === 'LOCAL' ? 'restart' : 'refresh')}>{t('refresh')}</button>
    {#if snapshot.state === 'LOCAL'}
      <button disabled={busy || !selectedPeer || !!pairing.job} onclick={() => run('connect')}>{t('connect')}</button>
    {:else if snapshot.state === 'FAULTED' || snapshot.state === 'UNKNOWN'}
      <button disabled={busy || !bridge} onclick={() => run('restart')}>{t('restart')}</button>
    {:else}
      <button disabled={busy || !canDisconnect} onclick={() => run('disconnect')}>{t('disconnect')}</button>
    {/if}
  </div>
  <section class="heading"><h1>{t('devices')}</h1><p>{t('help')}</p></section>
  {#if error}
    <p class="fault-banner" role="alert">{t(error)}</p>
  {/if}
  {#if inputNeedsAction || inputBusy || inputNotice === 'UPDATING' || inputNotice === 'PENDING' || snapshot.inputBackend === 'PORTABLE'}
    <p class="fault-banner" role="status">{t(inputNoticeKey)} <button disabled={busy || !bridge || inputNotice === 'UPDATING' || inputNotice === 'PENDING'} onclick={refreshInput}>{t(inputNotice === 'NOT_INSTALLED' ? 'input_setup' : 'input_refresh')}</button></p>
  {/if}
  {#if snapshot.boundaryBlocked}<p class="placement-hint" role="status">{t(`boundary_${snapshot.boundaryBlocked}`)}</p>{/if}
  <label class="peer-select">{t('peer')}<select bind:value={selected} disabled={busy || snapshot.state !== 'LOCAL'}><option value="">{t('choose')}</option>{#each snapshot.peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}</select></label>
  <PeerAppButton bridge={appOpenBridge} peer={selectedPeer} {locale} />
  <section class="canvas" aria-label={t('devices')}>
    <div class="screens" class:reversed={shownEdge === 'LEFT'}>
      <div class="screen"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong>{t('local')}</strong><span>{t('thisPC')}</span><small>{shownEdge ? t(shownEdge === 'LEFT' ? 'right' : 'left') : '—'}</small></div></div>
      <div class="screen peer"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong class="peer-name" dir="auto">{snapshot.peerAlias || selectedPeer?.alias || '—'}</strong><span>{t('otherPC')}</span><small>{shownEdge ? t(shownEdge === 'LEFT' ? 'left' : 'right') : '—'}</small></div></div>
    </div>
    <p class="placement-hint"><Icon name="info" size={20} />{t(snapshot.state === 'LOCAL' ? 'placementReady' : 'placementLocked')}</p>
    <div class="canvas-actions">
      <button disabled={!canPlace} onclick={swap}><Icon name="arrows-left-right" size={18} />{t('swap')}</button>
      <label class="rearm-field">{t('rearm')}
        <select disabled={!canPlace} value={layoutPeer === selected ? layout.rearm ?? '' : ''} onchange={e => setRearm(Number(e.currentTarget.value))}>
          <option value="" disabled>—</option>
          {#each [32,64,128] as value}<option {value}>{value}px</option>{/each}
        </select>
      </label>
      <button aria-haspopup="dialog" onclick={() => openDialog(pairingDlg)}><Icon name="plus" size={18} />{t('addDevice')}</button>
    </div>
  </section>
  <section class="utilities">
    <div class="utility utility-clipboard">
      <Icon name="clipboard" />
      <div class="utility-copy">
        <h2>{t('clipboard')}</h2>
        <span id="clipboard-state" class="utility-state" aria-live="polite">{t(!clipboardKnown ? 'clipboard_unknown' : clipboardEnabled ? 'clipboard_enabled' : 'clipboard_disabled')}{#if clipboardEnabled && clipboardAlias} · {clipboardAlias}{/if}</span>
				{#if fileTransfer}<span class="utility-state" aria-live="polite">{fileTransferLabel}{#if ['SENDING', 'RECEIVING'].includes(fileTransfer.state) && fileTransfer.total > 0} · {Math.floor(fileTransfer.bytes * 100 / fileTransfer.total)}%{/if}</span>{/if}
        {#if clipNotice}<small class="utility-error" role="alert" tabindex="-1" bind:this={clipError}>{t(clipNotice)}</small>{/if}
      </div>
      <button type="button" class="switch" role="switch" aria-checked={clipboardEnabled} aria-label={t('clipboard')} aria-describedby="clipboard-state" disabled={clipBusy || !clipboardKnown || (!clipboardEnabled && !selectedPeer)} onclick={toggleClipboard}><span></span></button>
    </div>
    <button class="utility utility-button" aria-haspopup="dialog" onclick={openUpdates}><Icon name="arrows-clockwise" /><div><h2>{t('update')}</h2></div></button>
    <button class="utility utility-button" aria-haspopup="dialog" onclick={() => openDialog(pairingDlg)}><Icon name="plus" /><div><h2>{t('pair')}</h2></div></button>
    <button class="utility utility-button" aria-haspopup="dialog" onclick={() => openDialog(diagnosticsDlg)}><Icon name="info" /><div><h2>{t('diag_title')}</h2></div></button>
  </section>
  {#if selectedPeer && layoutPeer === selected && layout.code !== 'ok' && layout.code !== 'unconfigured'}
    <p role="status">{t(layout.code)}</p>
  {/if}
</main>
<dialog bind:this={updatesDlg} aria-labelledby="update-settings-title" onclose={() => { updatesOpen = false; restoreDialogFocus(); }}>
  {#if updatesOpen}<UpdateSettings bridge={updateSettingsBridge} peers={snapshot.peers} initialPeer={selected} inputBackend={snapshot.inputBackend ?? 'UNKNOWN'} inputSetup={snapshot.inputSetup ?? 'UNKNOWN'} {inputBusy} {refreshInput} {locale} />{/if}
  <div class="dlg-foot"><button class="primary" onclick={() => updatesDlg.close()}>{t('close')}</button></div>
</dialog>
<dialog bind:this={diagnosticsDlg} aria-labelledby="diagnostics-title" onclose={restoreDialogFocus}>
  <div class="dlg-head"><h2 id="diagnostics-title">{t('diag_title')}</h2><p>{t('diag_help')}</p></div>
  <div class="dlg-body">
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
  </div>
  <div class="dlg-foot"><button class="primary" onclick={() => diagnosticsDlg.close()}>{t('close')}</button></div>
</dialog>
<dialog bind:this={consent} aria-labelledby="clipboard-consent-title" onclose={restoreDialogFocus}>
  <div class="dlg-head"><h2 id="clipboard-consent-title">{t('clipboard')}</h2></div>
  <div class="dlg-body"><p>{t('clipboard_consent')}</p></div>
  <div class="dlg-foot"><button onclick={()=>consent.close()}>{t('cancel')}</button><button onclick={()=>{consent.close();void setClipboard(consentPeer,true);}}>{t('clipboard_enable')}</button></div>
</dialog>
<dialog bind:this={pairingDlg} aria-labelledby="pair-title" onclose={restoreDialogFocus}>
  <div class="dlg-head"><h2 id="pair-title">{t('pairingTitle')}</h2><p>{t('pairingHelp')}</p></div>
  <div class="dlg-body">
  {#if pairing.pin}<p class="pin-display" role="status">{pairing.pin}</p>{/if}
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
  </div>
  <div class="dlg-foot"><button class="primary" onclick={() => pairingDlg.close()}>{t('close')}</button></div>
</dialog>
<footer><AppSettings {locale} /><select aria-label={t('language')} bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></footer>
<style>
  .peer-select { display:flex; align-items:center; gap:20px; margin:20px 0; flex-wrap:wrap; }
  .peer-select select, .peer-select input { min-width:min(220px,100%); max-width:100%; }
  .canvas { min-height: 0; }
  .screen { max-width:340px; }
  .screen-content strong { font-size:30px; }
  .peer-name { max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .fault-banner { font-size:14px; }
  .brand { white-space:nowrap; }
  .pin-display { font: 600 clamp(28px, 6vw, 42px)/1.2 ui-monospace, Consolas, monospace; letter-spacing: 0.3em; text-indent: 0.3em; text-align:center; margin: 12px 0 4px; max-width: 100%; overflow-wrap: anywhere; }
</style>
