<script>
  import { onMount } from 'svelte';
  import { emptyUpdateSettings, emptyUpdateTransfer, transferActive } from './update-settings-bridge.js';
  import { nativeText } from './native-i18n.js';
  let { bridge, peers, initialPeer = '', locale } = $props();
  let selected = $state('');
  let value = $state(emptyUpdateSettings());
  let busy = $state(false);
  let notice = $state('');
  let consent;
  let sendConsent;
  let sendPeer = $state('');
  let transfer = $state(emptyUpdateTransfer());
  let consentPeer = $state('');
  let disposed = false;
  const t = key => nativeText(locale, key);
  const current = $derived(value.peer === selected ? value : emptyUpdateSettings(selected));
  const job = $derived(transfer.peer === selected ? transfer : emptyUpdateTransfer(selected));
  const canSend = $derived(!!selected && !!bridge && !busy && current.peerState === 'ONLINE' && current.remotePermission === 'ALLOWED' && !transferActive(job.state));
  const selectedName = $derived(peers.find(peer => peer.fingerprint === selected)?.alias ?? '');
  const permissionText = permission => t(permission === 'ALLOWED' ? 'upd_allowed' : permission === 'DENIED' ? 'upd_denied' : 'upd_permission_unknown');
  const version = (generation, build) => generation ? `g${generation} · ${build ? build === 'development' ? build : build.slice(0,8) : t('upd_unknown')}` : t('upd_unknown');
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
  function choose() { value = emptyUpdateSettings(selected); transfer = emptyUpdateTransfer(selected); notice = ''; void run(); }
  function requestSend() { sendPeer = selected; sendConsent.showModal(); }
  async function confirmSend() {
    sendConsent.close();
    const peer = sendPeer;
    if (!canSend || selected !== peer || !peers.some(p => p.fingerprint === peer)) return;
    busy = true;
    transfer = emptyUpdateTransfer(peer,'PREPARING');
    try { const next = await bridge.send(peer); if (!disposed && selected === peer) transfer = next; }
    finally { if (!disposed) busy = false; }
  }
  function requestAllow() { consentPeer = selected; consent.showModal(); }
  function confirmAllow() {
    consent.close();
    if (consentPeer === selected && peers.some(peer => peer.fingerprint === consentPeer)) void run('allow', consentPeer);
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
  <p>{t('upd_help')}</p>
</div>
<div class="dlg-body">
  <label>{t('peer')}
    <select bind:value={selected} onchange={choose} disabled={busy}>
      <option value="">{t('choose')}</option>
      {#each peers as peer (peer.fingerprint)}<option value={peer.fingerprint}>{peer.alias}</option>{/each}
    </select>
  </label>
  <section class="upd-section" aria-label={t('upd_send')}>
    <h3>{t('upd_send')}</h3>
    <div class="upd-cards">
      <div class="upd-card"><strong>{t('upd_local_version')}</strong>{version(current.localGeneration,current.localBuild)}</div>
      <div class="upd-card"><strong>{t('upd_peer_version')}</strong>{version(current.peerGeneration,current.peerBuild)}</div>
    </div>
    <p>{t('upd_' + current.peerState.toLowerCase())} · {t('upd_comparison_' + current.comparison)}</p>
    <p>{t('upd_receiver_required')}</p>
    <div class="upd-send-row">
      <button class="primary" disabled={!canSend} onclick={requestSend}>{t('upd_send')}</button>
    </div>
    <p class="upd-job" role="status" aria-live="polite">{t('upd_state_' + job.state)}{#if job.state === 'SENDING' && job.total > 0} · {Math.floor(job.bytes*100/job.total)}%{/if}</p>
    {#if job.generation}<p class="upd-job">{t('upd_bundle_version')}: {version(job.generation,job.build)}</p>{/if}
    {#if job.state === 'SENDING' && job.total > 0}<div class="upd-job"><progress value={job.bytes} max={job.total} aria-label={t('upd_send')}></progress></div>{/if}
  </section>
  <section class="upd-section" aria-label={t('upd_permission')}>
    <h3>{t('upd_permission')}</h3>
    <p>{permissionText(current.permission)}</p>
    <p>{t('upd_remote_permission')}: {permissionText(current.remotePermission)}</p>
    <div class="pair-actions">
      <button disabled={!selected || busy || !bridge} onclick={() => run()}>{t('upd_refresh')}</button>
      <button disabled={!selected || busy || !bridge || current.permission === 'ALLOWED'} onclick={requestAllow}>{t('upd_allow')}</button>
      <button disabled={!selected || busy || !bridge || current.permission === 'DENIED'} onclick={() => run('deny')}>{t('upd_deny')}</button>
    </div>
  </section>
  <p role="status">{notice ? t(notice) : current.code === 'ok' ? '' : t('upd_' + current.code)}</p>
  <p>{t('upd_sending_later')}</p>
</div>
<dialog bind:this={sendConsent} aria-labelledby="send-update-title">
  <div class="dlg-head"><h2 id="send-update-title">{t('upd_send_confirm_title')}</h2></div>
  <div class="dlg-body"><p>{selectedName}</p><p>{t('upd_send_confirm')}</p></div>
  <div class="dlg-foot"><button onclick={() => sendConsent.close()}>{t('cancel')}</button><button class="primary" disabled={!canSend || sendPeer !== selected} onclick={confirmSend}>{t('upd_send')}</button></div>
</dialog>
<dialog bind:this={consent} aria-labelledby="update-consent-title">
  <div class="dlg-head"><h2 id="update-consent-title">{t('upd_consent_title')}</h2></div>
  <div class="dlg-body"><p>{selectedName}</p><p>{t('upd_consent')}</p></div>
  <div class="dlg-foot"><button onclick={() => consent.close()}>{t('cancel')}</button><button disabled={busy || consentPeer !== selected} onclick={confirmAllow}>{t('upd_confirm')}</button></div>
</dialog>
