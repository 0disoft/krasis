<script>
  import Icon from './Icon.svelte';
  import logo from '../../assets/windows/krasis.ico';
  import monitorArtwork from './assets/monitor.png';
  import { resolveLocale, translate } from './i18n.js';
  let saved;
  try { saved = localStorage.getItem('krasis.preview.locale'); } catch {}
  let locale = $state(resolveLocale(saved, navigator.languages));
  let tab = $state('workspace');
  let state = $state('CONNECTED');
  let peerRight = $state(true);
  let clipboard = $state(true);
  let rearm = $state(32);
  let feedback = $state(false);
  let pairing;
  const t = (key, values) => translate(locale, key, values);
  const tabs = [{ id: 'workspace', icon: 'monitor' }, { id: 'devices', icon: 'laptop' }, { id: 'settings', icon: 'gear-six' }];
  const previewAction = () => { feedback = true; };
  $effect(() => {
    document.documentElement.lang = locale;
    try { localStorage.setItem('krasis.preview.locale', locale); } catch {}
  });
</script>

<header>
  <a class="brand" href="#workspace" onclick={() => tab = 'workspace'}><img src={logo} alt="" />Krasis</a>
  <nav aria-label="Krasis">
    {#each tabs as item}<button class:active={tab === item.id} aria-current={tab === item.id ? 'page' : undefined} onclick={() => tab = item.id}><Icon name={item.icon} />{t(item.id)}</button>{/each}
  </nav>
  <span class="preview-label">{t('preview')}</span>
</header>

<main>
  <div class="connection-bar">
    <span class="connection" class:fault={state === 'FAULTED'}><span class="status-dot"></span>{t(state === 'CONNECTED' ? 'connected' : state === 'FAULTED' ? 'faulted' : 'local')}</span>
    <button onclick={() => { state = state === 'CONNECTED' ? 'LOCAL' : 'CONNECTED'; feedback = false; }}>{t(state === 'CONNECTED' ? 'disconnect' : state === 'FAULTED' ? 'retry' : 'connect')}</button>
  </div>
  {#if state === 'FAULTED'}<div class="fault-banner" role="status"><Icon name="warning-circle" />{t('faultHelp')}</div>{/if}

  {#if tab === 'workspace'}
    <section class="heading"><h1>{t('placement')}</h1><p>{t('placementHelp')}</p></section>
    <section class="canvas" aria-label={t('placement')}>
      <div class="screens" class:reversed={!peerRight}>
        <div class="screen"><img class="monitor-art" src={monitorArtwork} alt="" /><div class="screen-content"><strong>A</strong><span>{t('thisPC')}</span><small>{t(peerRight ? 'left' : 'right')}</small></div></div>
        <div class="screen peer"><img class="monitor-art" src={monitorArtwork} alt="" /><div class="screen-content"><strong>B</strong><span>{t('otherPC')}</span><small>{t(peerRight ? 'right' : 'left')}</small></div></div>
      </div>
      <div class="placement-hint"><Icon name="info" size={20} />{t(state === 'LOCAL' ? 'placementReady' : 'placementLocked')}</div>
      <div class="canvas-actions"><button disabled={state !== 'LOCAL'} onclick={() => peerRight = !peerRight}><Icon name="arrows-left-right" size={18} />{t('swap')}</button><button onclick={() => pairing.showModal()}><Icon name="plus" size={18} />{t('addDevice')}</button></div>
    </section>
    <section class="utilities">
      <div class="utility"><Icon name="clipboard" size={36} /><div><h2>{t('clipboard')}</h2><p>{t('clipboardHelp')}</p></div><button class="switch" role="switch" aria-checked={clipboard} aria-label={t('clipboard')} onclick={() => clipboard = !clipboard}><span></span></button></div>
      <button class="utility utility-button" onclick={previewAction}><Icon name="arrows-clockwise" size={36} /><div><h2>{t('update')}</h2><p>{t('updateHelp')}</p><small>{t('unverified')}</small></div></button>
      <button class="utility utility-button" onclick={previewAction}><Icon name="file-text" size={36} /><div><h2>{t('diagnostics')}</h2><p>{t('diagnosticsHelp')}</p></div></button>
    </section>
  {:else if tab === 'devices'}
    <section class="heading"><h1>{t('devices')}</h1><p>{t('deviceHelp')}</p></section>
    <section class="panel"><h2>{t('paired')}</h2><div class="device-row"><Icon name="laptop" size={48} /><div><h3 dir="auto">B</h3><p>{t('trusted')}</p></div><span class="tag">{t(peerRight ? 'right' : 'left')}</span></div><button onclick={() => pairing.showModal()}><Icon name="plus" />{t('addDevice')}</button></section>
  {:else}
    <section class="heading"><h1>{t('settings')}</h1><p>{t('settingsHelp')}</p></section>
    <section class="panel settings-panel">
      <div class="setting"><div><h2>{t('language')}</h2></div><select aria-label={t('language')} bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></div>
      <div class="setting"><div><h2>{t('rearm')}</h2><p>{t('rearmHelp')}</p></div><select disabled={state !== 'LOCAL'} aria-label={t('rearm')} bind:value={rearm}>{#each [32,64,128] as value}<option {value}>{t('rearmValue', {value: new Intl.NumberFormat(locale).format(value)})}</option>{/each}</select></div>
      <div class="setting"><div><h2>{t('automatic')}</h2><p>{t('automaticHelp')}</p></div><Icon name="shield-check" size={28} /></div>
      <div class="setting"><div><h2>{t('version')}</h2><p>{t('versionHelp')}</p></div><span class="tag">{t('notConnected')}</span></div>
    </section>
  {/if}
  <p class="feedback" role="status">{feedback ? t('previewAction') : ''}</p>
</main>
<footer><span class="permission"><Icon name="shield-check" size={18} />{t('permission')}</span><label><Icon name="globe" size={20} /><select aria-label={t('language')} bind:value={locale}><option value="ko">한국어</option><option value="en">English</option></select></label></footer>
<aside class="preview-tools"><span>{t('previewHelp')}</span><label>{t('scenario')}<select bind:value={state}><option value="CONNECTED">{t('connectedScenario')}</option><option value="LOCAL">{t('localScenario')}</option><option value="FAULTED">{t('faultScenario')}</option></select></label></aside>

<dialog bind:this={pairing} aria-labelledby="pair-title"><form method="dialog"><h2 id="pair-title">{t('pairingTitle')}</h2><p>{t('pairingHelp')}</p><div class="pair-actions"><button type="button" disabled>{t('createPIN')}</button><button type="button" disabled>{t('findDevice')}</button></div><p class="muted">{t('pairingPreview')}</p><button class="primary">{t('close')}</button></form></dialog>
