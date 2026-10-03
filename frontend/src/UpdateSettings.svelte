<script>
  import { onMount, tick } from 'svelte';
  import { emptyUpdateSettings, emptyUpdateTransfer, transferActive } from './update-settings-bridge.js';
  import { nativeText } from './native-i18n.js';
  let { bridge, peers, initialPeer = '', inputBackend = 'UNKNOWN', locale } = $props();
  let selected = $state('');
  let value = $state(emptyUpdateSettings());
  let busy = $state(false);
  let notice = $state('');
  let sendTrigger = $state.raw();
  let allowTrigger;
  let refreshTrigger;
  let peerSelect = $state.raw();
  let initialFocusPending = true;
  let sendConfirmation = $state.raw();
  let allowConfirmation = $state.raw();
  let confirmingSend = $state(false);
  let confirmingAllow = $state(false);
  let sendPeer = $state('');
  let transfer = $state(emptyUpdateTransfer());
  let consentPeer = $state('');
  let disposed = false;
  const t = key => nativeText(locale, key);
  const current = $derived(value.peer === selected ? value : emptyUpdateSettings(selected));
  const job = $derived(transfer.peer === selected ? transfer : emptyUpdateTransfer(selected));
  const canSend = $derived(!!selected && !!bridge && !busy && current.appGeneration > 0 && current.localGeneration === current.appGeneration && current.peerState === 'ONLINE' && current.remotePermission === 'ALLOWED' && !transferActive(job.state));
  const selectedName = $derived(peers.find(peer => peer.fingerprint === selected)?.alias ?? '');
  const permissionText = permission => t(permission === 'ALLOWED' ? 'upd_allowed' : permission === 'DENIED' ? 'upd_denied' : 'upd_permission_unknown');
  const version = (generation, build) => generation ? `g${generation} · ${build ? build === 'development' ? build : build.slice(0,8) : t('upd_unknown')}` : t('upd_unknown');
  $effect(() => {
    if (busy || !initialFocusPending || !peerSelect) return;
    const dialog = peerSelect.closest('dialog');
    if (!dialog?.open) return;
    initialFocusPending = false;
    const close = dialog.querySelector('.dlg-foot button');
    if (document.activeElement === close || document.activeElement === dialog) peerSelect.focus();
  });
  $effect(() => {
    if (!peers.some(peer => peer.fingerprint === selected)) selected = (peers.find(peer => peer.fingerprint === initialPeer) ?? peers[0])?.fingerprint ?? '';
  });
  async function run(action = 'status', peer = selected) {
    if (!bridge || !peer || busy || disposed) return;
    busy = true;
    if (action !== 'status') notice = '';
    try {
      const result = await (action === 'status' ? bridge.status(peer) : bridge.permit(peer, action === 'allow'));
      if (disposed || selected !== peer) return;
      value = result;
      if (action === 'status') {
        const next = await bridge.transfer(peer);
        if (!disposed && selected === peer) transfer = next;
      }
      if (action !== 'status') notice = 'upd_' + result.code;
    } finally { if (!disposed) busy = false; }
  }
  function choose() { confirmingSend = false; confirmingAllow = false; value = emptyUpdateSettings(selected); transfer = emptyUpdateTransfer(selected); notice = ''; void run(); }
  async function requestSend() { sendPeer = selected; confirmingAllow = false; confirmingSend = true; await tick(); sendConfirmation?.querySelector('button')?.focus(); }
  async function cancelSend() { confirmingSend = false; await tick(); sendTrigger?.focus(); }
  async function confirmSend() {
    const peer = sendPeer;
    if (!canSend || selected !== peer || !peers.some(p => p.fingerprint === peer)) return;
    confirmingSend = false;
    busy = true;
    transfer = emptyUpdateTransfer(peer,'PREPARING');
    try { const next = await bridge.send(peer); if (!disposed && selected === peer) transfer = next; }
    finally { if (!disposed) { busy = false; await tick(); peerSelect?.focus(); } }
  }
  async function requestAllow() { consentPeer = selected; confirmingSend = false; confirmingAllow = true; await tick(); allowConfirmation?.querySelector('button')?.focus(); }
  async function cancelAllow() { confirmingAllow = false; await tick(); allowTrigger?.focus(); }
  async function confirmAllow() {
    if (busy || consentPeer !== selected || !peers.some(peer => peer.fingerprint === consentPeer)) return;
    confirmingAllow = false;
    if (current.permission !== 'ALLOWED') await run('allow', consentPeer);
    await tick(); refreshTrigger?.focus();
  }
  onMount(() => {
    let timer;
    const poll = async () => { await run(); if (!disposed) timer = setTimeout(poll,1500); };
    void poll();
    return () => { disposed = true; clearTimeout(timer); };
  });
</script>

<div class="dlg-head">
  <h2 id="update-settings-title">{t('upd_title')}</h2>
</div>
<div class="dlg-body">
  <section class="upd-section" aria-label={t('input_title')}>
    <h3>{t('input_title')}</h3>
    <p role="status">{t('input_' + inputBackend.toLowerCase())}</p>
    {#if inputBackend !== 'PROTECTED'}<p>{t('input_install_help')}</p>{/if}
  </section>
  <label>{t('peer')}
    <select bind:this={peerSelect} bind:value={selected} onchange={choose} disabled={busy}>
      <option value="">{t('choose')}</option>
      {#each peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}
    </select>
  </label>
  <section class="upd-section" aria-label={t('upd_send')}>
    <div class="upd-cards">
      <div class="upd-card"><strong>{t('upd_local_version')}</strong>{version(current.localGeneration,current.localBuild)}</div>
      <div class="upd-card"><strong>{t('upd_peer_version')}</strong>{version(current.peerGeneration,current.peerBuild)}</div>
    </div>
    {#if current.appGeneration && current.localGeneration && current.appGeneration !== current.localGeneration}<p role="status">{t('upd_agent_mismatch')}</p>{/if}
    <p>{t('upd_' + current.peerState.toLowerCase())} · {t('upd_comparison_' + current.comparison)}</p>
    {#if current.peerState !== 'ONLINE'}<p>{t('upd_receiver_required')}</p>{/if}
    {#if current.peerState === 'ONLINE' && current.peerGeneration < 75}<p>{t('upd_transition_first')}</p>{/if}
    {#if current.peerState === 'ONLINE' && current.peerGeneration === 75}<p>{t('upd_transition_next')}</p>{/if}
    {#if !confirmingSend}
      <div class="upd-send-row">
        <button bind:this={sendTrigger} class="primary" disabled={!canSend || confirmingAllow} onclick={requestSend}>{t('upd_send')}</button>
      </div>
    {/if}
    {#if confirmingSend}
      <section class="upd-confirm" aria-labelledby="send-update-title" bind:this={sendConfirmation}>
        <h3 id="send-update-title">{t('upd_send_confirm_title')}</h3>
        <p>{selectedName}</p><p>{t(current.peerGeneration < 75 ? 'upd_send_transition_confirm' : 'upd_send_confirm')}</p>
        <div class="pair-actions"><button onclick={cancelSend}>{t('cancel')}</button><button class="primary" disabled={!canSend || sendPeer !== selected} onclick={confirmSend}>{t('upd_send_confirm_action')}</button></div>
      </section>
    {/if}
    {#if job.state !== 'IDLE'}<p class="upd-job" role="status" aria-live="polite">{t('upd_state_' + job.state)}{#if job.state === 'SENDING' && job.total > 0} · {Math.floor(job.bytes*100/job.total)}%{/if}</p>{/if}
    {#if job.generation}<p class="upd-job">{t('upd_bundle_version')}: {version(job.generation,job.build)}</p>{/if}
    {#if job.state === 'SENDING' && job.total > 0}<div class="upd-job"><progress value={job.bytes} max={job.total} aria-label={t('upd_send')}>{Math.floor(job.bytes*100/job.total)}%</progress></div>{/if}
  </section>
  <section class="upd-section" aria-label={t('upd_permission')}>
    <h3>{t('upd_permission')}</h3>
    <p>{permissionText(current.permission)}</p>
    <p>{t('upd_remote_permission')}: {permissionText(current.remotePermission)}</p>
    <div class="pair-actions">
      <button bind:this={refreshTrigger} disabled={!selected || busy || !bridge} onclick={() => run()}>{t('upd_refresh')}</button>
      <button bind:this={allowTrigger} disabled={!selected || busy || !bridge || current.permission === 'ALLOWED' || confirmingSend || confirmingAllow} onclick={requestAllow}>{t('upd_allow')}</button>
      <button disabled={!selected || busy || !bridge || current.permission === 'DENIED'} onclick={() => run('deny')}>{t('upd_deny')}</button>
    </div>
    {#if confirmingAllow}
      <section class="upd-confirm" aria-labelledby="update-consent-title" bind:this={allowConfirmation}>
        <h3 id="update-consent-title">{t('upd_consent_title')}</h3>
        <p>{selectedName}</p><p>{t('upd_consent')}</p>
        <div class="pair-actions"><button onclick={cancelAllow}>{t('cancel')}</button><button disabled={busy || consentPeer !== selected || current.permission === 'ALLOWED'} onclick={confirmAllow}>{t('upd_confirm')}</button></div>
      </section>
    {/if}
  </section>
  {#if notice || current.code !== 'ok'}<p role="status">{notice ? t(notice) : t('upd_' + current.code)}</p>{/if}
</div>
<style>
  .upd-confirm { margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--line); }
  @media (max-width: 700px) {
    .upd-cards { grid-template-columns: 1fr; }
  }
</style>
