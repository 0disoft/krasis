<script>
  import { onMount, tick } from 'svelte';
  import UpdateSettings from './UpdateSettings.svelte';
  import AppSettings from './AppSettings.svelte';
  import KeyboardPanel from './KeyboardPanel.svelte';
  import PeerAppButton from './PeerAppButton.svelte';
  import { createAppOpenBridge } from './app-open-bridge.js';
  import { createUpdateSettingsBridge } from './update-settings-bridge.js';
  import Icon from './Icon.svelte';
  import WindowTitlebar from './WindowTitlebar.svelte';
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
  import { createTrayLocaleSync } from './tray-locale.js';
  let saved;
  try { saved = localStorage.getItem('krasis.workspace.locale'); } catch {}
  let locale = $state(resolveLocale(saved, navigator.languages));
  let syncTrayLocale = $state.raw();
  let trayLocaleFailed = $state(false);
  let snapshot = $state({state:'UNKNOWN', peerAlias:'', peers:[], code:'unavailable', inputSetup:'UNKNOWN'});
  let selected = $state('');
  let busy = $state(false);
  let windowRuntime = $state.raw();
  let windowVersion = $state('');
  let windowMaximised = $state(false);
  let windowFocused = $state(true);
  let windowBusy = false;
  let windowStateEpoch = 0;
  let windowEvents = [];
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
  let diagFolderFailed = $state(false);
  let diagnosticsDlg;
  let updateSettingsBridge = $state.raw();
  let appOpenBridge = $state.raw();
  let updatesDlg;
  let settingsDlg;
  let settingsOpen = $state(false);
  let appSettingsCode = $state('ok');
  let releaseState = $state('IDLE');
  let updatesOpen = $state(false);
  let opener = null;
  function focusFirstControl(dlg) {
    const controls = dlg?.querySelectorAll('select:not([disabled]), input:not([disabled]), summary, button:not([disabled]):not([data-window-control])') ?? [];
    [...controls].find(control => control.getClientRects().length > 0)?.focus();
  }
  function openDialog(dlg) { if (!dlg || dlg.open) return; opener = document.activeElement; dlg.showModal(); if (dlg === settingsDlg) settingsOpen = true; focusFirstControl(dlg); }
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
  async function refreshWindowState() {
    if (!windowRuntime || disposed) return;
    const epoch = ++windowStateEpoch;
    try {
      const [maximised, focused] = await Promise.all([windowRuntime.IsMaximised(), windowRuntime.IsFocused()]);
      if (!disposed && epoch === windowStateEpoch) { windowMaximised = maximised === true; windowFocused = focused === true; }
    } catch { /* Window state is optional; never refresh or restart the renderer. */ }
  }
  async function windowAction(action) {
    if (!windowRuntime || windowBusy || disposed) return;
    const method = {minimise:'Minimise',maximise:'ToggleMaximise',close:'Close'}[action];
    if (!method) return;
    windowBusy = true;
    try { await windowRuntime[method](); await refreshWindowState(); }
    catch { if (!disposed) error = 'window_control_failed'; }
    finally { windowBusy = false; }
  }
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
  async function runDiagnostics(action, background = false) {
    if (!diagnosticsBridge || diagBusy || disposed) return;
    await diagnosticsOperation(async () => {
      if (disposed) return;
      diagAction = action !== 'status';
      try {
        const value = await (action === 'status' ? diagnosticsBridge.status() :
          action === 'open' ? diagnosticsBridge.open() : diagnosticsBridge.collect());
        if (disposed) return;
        diagnostics = value;
        if (action === 'open') diagFolderFailed = value.code !== 'ok';
        else if (action === 'collect' || action === 'status' && value.collection) {
          diagNotice = 'diag_' + value.code;
          if (action === 'collect') diagFolderFailed = false;
        }
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
      if (!disposed && !diagBusy && diagnosticsBridge) await runDiagnostics('status', true);
      if (!disposed) timer = setTimeout(poll, 1500);
    }
    void (async () => {
      try {
        const { Call, Window, Events } = await import(/* @vite-ignore */ '/wails/runtime.js');
        if (disposed) return;
        const name = 'github.com/0disoft/krasis/internal/workspacehost.WorkspaceService.';
        syncTrayLocale = createTrayLocaleSync(value => Call.ByName(name+'SetTrayLocale', value), failed => { if (!disposed) trayLocaleFailed = failed; });
        windowRuntime = Window;
        for (const event of ['common:WindowMaximise','common:WindowUnMaximise','common:WindowFocus','common:WindowLostFocus']) {
          windowEvents.push(Events.On(event, refreshWindowState));
        }
        void refreshWindowState();
        void Call.ByName(name+'WindowVersion').then(version => {
          if (!disposed && Number.isSafeInteger(version) && version > 0) windowVersion = 'g'+version;
        }).catch(() => {});
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
          DiagnosticsCollect:()=>Call.ByName(name+'DiagnosticsCollect'),
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
    return () => { disposed = true; clearTimeout(timer); windowEvents.forEach(off => off()); };
  });
  $effect(() => {
    document.documentElement.lang = locale;
    try { localStorage.setItem('krasis.workspace.locale', locale); } catch {}
    if (syncTrayLocale) void syncTrayLocale(locale);
  });
</script>

<WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
<main class="workspace">
  <h1 class="sr-only">{t('workspace')}</h1>
  <section class="workspace-toolbar" aria-label={t('devices')}>
    <div class="device-controls">
      <label class="device-select">{t('peer')}<select bind:value={selected} disabled={busy || snapshot.state !== 'LOCAL'}><option value="">{t('choose')}</option>{#each snapshot.peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}</select></label>
      <PeerAppButton bridge={appOpenBridge} peer={selectedPeer} {locale} compact />
      {#if snapshot.peers.length}<button class="icon-button" aria-label={t('pair')} title={t('pair')} aria-haspopup="dialog" onclick={() => openDialog(pairingDlg)}><Icon name="plus" size={18} /></button>{/if}
    </div>
    <div class="connection-controls">
    <span class="connection" class:fault={snapshot.state !== 'CONNECTED'} role="status" aria-atomic="true"><span class="status-dot"></span>{t(snapshot.state)}</span>
    <button class="icon-button" aria-label={t('refresh')} title={t('refresh')} disabled={busy || !bridge} onclick={() => run(snapshot.state === 'LOCAL' ? 'restart' : 'refresh')}><Icon name="arrows-clockwise" size={18} /></button>
    {#if snapshot.state === 'LOCAL'}
      <button class="primary" disabled={busy || !selectedPeer || !!pairing.job} onclick={() => run('connect')}>{t('connect')}</button>
    {:else if snapshot.state === 'FAULTED' || snapshot.state === 'UNKNOWN'}
      <button disabled={busy || !bridge} onclick={() => run('restart')}>{t('restart')}</button>
    {:else}
      <button disabled={busy || !canDisconnect} onclick={() => run('disconnect')}>{t('disconnect')}</button>
    {/if}
    </div>
  </section>
  {#if error}
    <p class="fault-banner" role="alert">{t(error)}</p>
  {/if}
  {#if inputNeedsAction || inputBusy || inputNotice === 'UPDATING' || inputNotice === 'PENDING' || snapshot.inputBackend === 'PORTABLE'}
    <p class="fault-banner" role="status">{t(inputNoticeKey)} <button disabled={busy || !bridge || inputNotice === 'UPDATING' || inputNotice === 'PENDING'} onclick={refreshInput}>{t(inputNotice === 'NOT_INSTALLED' ? 'input_setup' : 'input_refresh')}</button></p>
  {/if}
  {#if snapshot.boundaryBlocked}<p class="placement-hint" role="status">{t(`boundary_${snapshot.boundaryBlocked}`)}</p>{/if}
  {#if appSettingsCode !== 'ok'}<p class="fault-banner" role="alert">{t(['startup_failed','start_menu_failed','recovery_failed','save_failed'].includes(appSettingsCode) ? `settings_${appSettingsCode}` : 'settings_failed')} <button onclick={() => openDialog(settingsDlg)}>{t('settings')}</button></p>{/if}
  {#if ['CHECKING','AVAILABLE','DOWNLOADING','READY','FAILED'].includes(releaseState)}<p class="release-notice" role="status">{t(`release_${releaseState}`)}</p>{/if}
  <section class="canvas workspace-canvas" aria-label={t('devices')}>
    {#if snapshot.state === 'LOCAL' && snapshot.peers.length === 0}
      <div class="empty-workspace"><Icon name="laptop" size={40} /><h2>{t('pair')}</h2><p>{t('pairHelp')}</p><button class="primary" aria-haspopup="dialog" onclick={() => openDialog(pairingDlg)}><Icon name="plus" size={18} />{t('addDevice')}</button></div>
    {:else}
    <div class="screens" class:reversed={shownEdge === 'LEFT'}>
      <div class="screen"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong>{t('local')}</strong>{#if shownEdge}<span>{t(shownEdge === 'LEFT' ? 'right' : 'left')}</span>{/if}</div></div>
      <div class="screen peer"><img class="monitor-art" src={monitor} alt="" /><div class="screen-content"><strong class="peer-name" dir="auto">{snapshot.peerAlias || selectedPeer?.alias || '—'}</strong>{#if shownEdge}<span>{t(shownEdge === 'LEFT' ? 'left' : 'right')}</span>{/if}</div></div>
    </div>
    {/if}
  </section>
  <section class="workspace-actions" aria-label={t('workspace_actions')}>
    <div class="clipboard-control">
      <Icon name="clipboard" size={20} />
      <div class="clipboard-copy">
        <span class="clipboard-label">{t('clipboard')}</span>
        <span id="clipboard-state" class="utility-state" aria-live="polite">{t(!clipboardKnown ? 'clipboard_unknown' : clipboardEnabled ? 'clipboard_enabled' : 'clipboard_disabled')}{#if clipboardEnabled && clipboardAlias} · {clipboardAlias}{/if}</span>
      </div>
      <button type="button" class="switch" role="switch" aria-checked={clipboardEnabled} aria-label={t('clipboard')} aria-describedby="clipboard-state" disabled={clipBusy || !clipboardKnown || (!clipboardEnabled && !selectedPeer)} onclick={toggleClipboard}><span></span></button>
    </div>
    <div class="utility-actions">
      <button aria-haspopup="dialog" onclick={openUpdates}><Icon name="arrows-clockwise" size={18} />{t('update')}</button>
      <button aria-haspopup="dialog" onclick={() => openDialog(diagnosticsDlg)}><Icon name="info" size={18} />{t('diag_title')}</button>
      <button aria-haspopup="dialog" onclick={() => openDialog(settingsDlg)}><Icon name="gear-six" size={18} />{t('settings')}</button>
    </div>
  </section>
  {#if fileTransfer && ['PREPARING','QUEUED','SENDING','RECEIVING','READY','FAILED'].includes(fileTransfer.state)}<p class="transfer-notice" role="status">{fileTransferLabel}{#if ['SENDING', 'RECEIVING'].includes(fileTransfer.state) && fileTransfer.total > 0} · {Math.floor(fileTransfer.bytes * 100 / fileTransfer.total)}%{/if}</p>{/if}
  {#if clipNotice}<p class="transfer-notice" role="alert" tabindex="-1" bind:this={clipError}>{t(clipNotice)}</p>{/if}
  {#if selectedPeer && layoutPeer === selected && layout.code !== 'ok' && layout.code !== 'unconfigured'}
    <p role="status">{t(layout.code)}</p>
  {/if}
</main>
<dialog class="workspace-settings" bind:this={settingsDlg} aria-labelledby="workspace-settings-title" onclose={() => { settingsOpen = false; restoreDialogFocus(); }}>
  <WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
  <div class="dlg-head settings-heading"><h2 id="workspace-settings-title">{t('settings')}</h2><span>Krasis {windowVersion}</span></div>
  <div class="dlg-body settings-body">
    <section class="settings-section">
      <h3>{t('placement_settings')}</h3>
      <div class="placement-settings">
        <button disabled={!canPlace} onclick={swap}><Icon name="arrows-left-right" size={18} />{t('swap')}</button>
        <label class="rearm-field">{t('rearm')}<select disabled={!canPlace} value={layoutPeer === selected ? layout.rearm ?? '' : ''} onchange={e => setRearm(Number(e.currentTarget.value))}><option value="" disabled>—</option>{#each [32,64,128] as value}<option {value}>{value}px</option>{/each}</select></label>
      </div>
      {#if snapshot.state !== 'LOCAL'}<p>{t('placementLocked')}</p>{/if}
    </section>
    <section class="settings-section"><KeyboardPanel peer={selected} {locale} active={settingsOpen} /></section>
    <section class="settings-section"><h3>{t('app_settings')}</h3><AppSettings {locale} active={settingsOpen} onStatus={(code, state) => { appSettingsCode = code; releaseState = state; }} /></section>
    <section class="settings-section"><label class="language-setting">{t('language')}<select bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></label>{#if trayLocaleFailed}<p role="status">{t('tray_locale_failed')}</p>{/if}</section>
  </div>
  <div class="dlg-foot"><button class="primary" onclick={() => settingsDlg.close()}>{t('close')}</button></div>
</dialog>
<dialog bind:this={updatesDlg} aria-labelledby="update-settings-title" onclose={() => { updatesOpen = false; restoreDialogFocus(); }}>
  <WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
  {#if updatesOpen}<UpdateSettings bridge={updateSettingsBridge} peers={snapshot.peers} initialPeer={selected} {inputBusy} {locale} />{/if}
  <div class="dlg-foot"><button class="primary" onclick={() => updatesDlg.close()}>{t('close')}</button></div>
</dialog>
<dialog bind:this={diagnosticsDlg} aria-labelledby="diagnostics-title" onclose={restoreDialogFocus}>
  <WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
  <div class="dlg-head"><h2 id="diagnostics-title">{t('diag_title')}</h2><p>{t('diag_help')}</p></div>
  <div class="dlg-body">
    <div class="pair-actions">
      <button class="primary" disabled={diagBusy || !diagnosticsBridge || ['SAVING','PENDING'].includes(diagnostics.collection?.state)} onclick={() => runDiagnostics('collect')}>{t(['SAVING','PENDING'].includes(diagnostics.collection?.state) ? 'diag_collecting' : 'diag_save')}</button>
      <button disabled={diagBusy || !diagnosticsBridge} onclick={() => runDiagnostics('open')}>{t('diag_open')}</button>
    </div>
    <p role="status">{#if !diagAction && diagnostics.collection?.localSaved && !['pending','collected','local_only','partial'].includes(diagnostics.code)}{t('diag_local_saved')} {/if}{diagAction ? t('diag_busy') : diagNotice ? t(diagNotice) : diagnostics.code !== 'ok' ? t('diag_' + diagnostics.code) : ''}</p>
    {#if diagFolderFailed}<p role="alert">{t('diag_open_failed')}</p>{/if}
  </div>
  <div class="dlg-foot"><button class="primary" onclick={() => diagnosticsDlg.close()}>{t('close')}</button></div>
</dialog>
<dialog bind:this={consent} aria-labelledby="clipboard-consent-title" onclose={restoreDialogFocus}>
  <WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
  <div class="dlg-head"><h2 id="clipboard-consent-title">{t('clipboard')}</h2></div>
  <div class="dlg-body"><p>{t('clipboard_consent')}</p></div>
  <div class="dlg-foot"><button onclick={()=>consent.close()}>{t('cancel')}</button><button onclick={()=>{consent.close();void setClipboard(consentPeer,true);}}>{t('clipboard_enable')}</button></div>
</dialog>
<dialog bind:this={pairingDlg} aria-labelledby="pair-title" onclose={restoreDialogFocus}>
  <WindowTitlebar {locale} version={windowVersion} maximised={windowMaximised} focused={windowFocused} ready={!!windowRuntime} action={windowAction} />
  <div class="dlg-head"><h2 id="pair-title">{t('pairingTitle')}</h2><p>{t('pairingHelp')}</p></div>
  <div class="dlg-body">
  {#if canDisconnect}<p role="status">{t('pairingLocked')}</p>{/if}
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
<style>
  :global(body) { padding-top:48px; }
  .sr-only { position:absolute; width:1px; height:1px; margin:-1px; padding:0; overflow:hidden; clip-path:inset(50%); white-space:nowrap; }
  .workspace { max-width:1120px; padding:24px 28px; }
  .workspace-toolbar { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:20px; }
  .device-controls, .connection-controls { display:flex; align-items:center; gap:8px; min-width:0; }
  .device-controls { flex-wrap:wrap; }
  .connection-controls { flex-shrink:0; }
  .device-select { display:flex; align-items:center; gap:10px; color:var(--muted); font-size:13px; }
  .device-select select { width:160px; font-size:14px; min-width:0; }
  .workspace button { font-size:14px; min-height:36px; padding:8px 12px; }
  .workspace .icon-button { width:36px; padding:0; flex-shrink:0; }
  .connection { gap:7px; font-size:13px; margin-inline-end:8px; }
  .status-dot { width:8px; height:8px; }
  .workspace-canvas { padding:28px 24px; min-height:0; background:#1d2027; border-color:#30343e; border-radius:16px; }
  .workspace-canvas::before { display:none; }
  .screens { gap:32px; }
  .screen { width:38%; max-width:280px; }
  .screen-content { gap:8px; }
  .screen-content strong { font-size:26px; font-weight:600; }
  .screen-content span { font-size:12px; color:var(--muted); }
  .workspace-actions { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-top:16px; padding:8px 0; }
  .clipboard-control { display:flex; align-items:center; gap:10px; min-width:0; }
  .clipboard-copy { display:flex; align-items:center; gap:8px; min-width:0; }
  .clipboard-label { font-size:14px; font-weight:600; white-space:nowrap; }
  .utility-state { font-size:12px; }
  .clipboard-control .switch { flex:0 0 50px; width:50px; margin:0; padding:0; min-height:36px; height:36px; }
  .switch::before { width:42px; height:26px; top:5px; }
  .switch span { width:20px; height:20px; top:8px; }
  .switch[aria-checked=true] span { transform:translateX(16px); }
  .utility-actions { display:flex; align-items:center; gap:8px; }
  .utility-actions button { border-color:transparent; background:transparent; color:#c5c8d4; }
  .utility-actions button:hover { background:#292d36; }
  .transfer-notice, .release-notice { font-size:13px; margin-top:8px; }
  .empty-workspace { display:flex; align-items:center; justify-content:center; flex-direction:column; gap:12px; min-height:200px; text-align:center; }
  .empty-workspace h2 { font-size:20px; margin:0; }
  .empty-workspace p { font-size:14px; }
  .workspace-settings { width:min(560px,calc(100vw - 32px)); max-height:calc(100dvh - 80px); }
  .settings-heading { display:flex; align-items:center; justify-content:space-between; gap:16px; }
  .settings-heading span { font-size:12px; color:var(--muted); }
  .settings-body { max-height:calc(100dvh - 220px); padding-block:0; }
  .settings-section { padding:18px 0; }
  .settings-section + .settings-section { border-top:1px solid var(--line); }
  .settings-section h3 { font-size:14px; font-weight:600; margin:0 0 12px; }
  .settings-section p { font-size:12px; margin-top:8px; }
  .settings-section :global(.keyboard-panel) { margin:0; }
  .placement-settings { display:flex; align-items:center; flex-wrap:wrap; gap:12px; }
  .placement-settings button { padding:8px 12px; font-size:13px; }
  .language-setting { display:flex; align-items:center; justify-content:space-between; gap:16px; font-size:14px; }
  .language-setting select { min-width:150px; }
  @media (max-width:820px) {
    .workspace { padding:16px; }
    .workspace-toolbar { gap:10px; margin-bottom:16px; }
    .device-select { gap:6px; font-size:12px; }
    .device-select select { width:125px; }
    .connection { margin-inline-end:0; }
    .workspace-canvas { padding:24px 16px; }
    .screens { gap:20px; }
    .screen-content strong { font-size:24px; }
    .clipboard-copy { gap:6px; }
    .utility-actions { gap:2px; }
    .workspace-actions { gap:8px; }
  }
  @media (max-width:650px) {
    .workspace-toolbar, .workspace-actions { flex-wrap:wrap; }
    .connection-controls, .utility-actions { margin-inline-start:auto; }
    .clipboard-copy { flex-wrap:wrap; }
  }
  .peer-select { display:flex; align-items:center; gap:20px; margin:20px 0; flex-wrap:wrap; }
  .peer-select select, .peer-select input { min-width:min(220px,100%); max-width:100%; }
  .peer-name { max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .fault-banner { font-size:14px; }
  .pin-display { font: 600 clamp(28px, 6vw, 42px)/1.2 ui-monospace, Consolas, monospace; letter-spacing: 0.3em; text-indent: 0.3em; text-align:center; margin: 12px 0 4px; max-width: 100%; overflow-wrap: anywhere; }
</style>
