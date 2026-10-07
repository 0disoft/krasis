<script>
  import logo from '../../assets/windows/krasis.ico';
  import Icon from './Icon.svelte';
  import { nativeText } from './native-i18n.js';
  let { locale, version = '', maximised = false, focused = true, ready = false, action } = $props();
  const t = key => nativeText(locale, key);
</script>

<header class="window-titlebar" class:inactive={!focused}>
  <div class="drag-region" ondblclick={() => ready && action('maximise')} role="presentation">
    <span class="window-brand"><img src={logo} alt="" draggable="false" />Krasis</span>
    {#if version}<span class="build-label">{version}</span>{/if}
    <span class="workspace-label"><Icon name="monitor" size={18} />{t('workspace')}</span>
    <span class="integration-label">{t('integration')}</span>
  </div>
  <div class="window-controls">
    <button data-window-control type="button" disabled={!ready} aria-label={t('window_minimise')} title={t('window_minimise')} onclick={() => action('minimise')}>
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8.5h10" /></svg>
    </button>
    <button data-window-control type="button" disabled={!ready} aria-label={t(maximised ? 'window_restore' : 'window_maximise')} title={t(maximised ? 'window_restore' : 'window_maximise')} onclick={() => action('maximise')}>
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
        {#if maximised}<path d="M5.5 5.5v-3h8v8h-3M2.5 5.5h8v8h-8z" />{:else}<path d="M3.5 3.5h9v9h-9z" />{/if}
      </svg>
    </button>
    <button data-window-control type="button" class="window-close" disabled={!ready} aria-label={t('window_close')} title={t('window_close')} onclick={() => action('close')}>
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg>
    </button>
  </div>
</header>

<style>
  .window-titlebar { position:fixed; inset:0 0 auto; z-index:10; height:56px; min-height:56px; padding:0; gap:0; background:#20232b; border-bottom:1px solid var(--line); user-select:none; }
  .drag-region { display:flex; align-items:center; align-self:stretch; flex:1; min-width:0; gap:12px; padding-inline-start:20px; --wails-draggable:drag; }
  .window-brand { display:flex; align-items:center; gap:10px; flex-shrink:0; font-size:19px; font-weight:600; letter-spacing:-.3px; color:#eeeef5; }
  .window-brand img { width:26px; height:26px; }
  .build-label { color:var(--muted); font:12px/1.2 ui-monospace,Consolas,monospace; padding:4px 7px; border:1px solid var(--line); border-radius:5px; }
  .workspace-label { display:flex; align-items:center; align-self:stretch; gap:8px; padding-inline:16px; margin-inline-start:16px; border-bottom:2px solid var(--accent); color:#c7a6ff; background:#2a293a; font-size:14px; white-space:nowrap; }
  .integration-label { margin-inline-start:auto; margin-inline-end:20px; color:var(--muted); font-size:11px; white-space:nowrap; }
  .window-controls { display:flex; align-self:stretch; flex-shrink:0; --wails-draggable:no-drag; }
  .window-controls button { width:46px; padding:0; border:0; border-radius:0; background:transparent; color:#d9dbe6; --wails-draggable:no-drag; }
  .window-controls button:hover:not(:disabled) { background:#353944; }
  .window-controls .window-close:hover:not(:disabled) { background:#c5364c; color:white; }
  .window-controls button:focus-visible { outline-offset:-4px; }
  svg { fill:none; stroke:currentColor; stroke-width:1.2; pointer-events:none; }
  .inactive .window-brand, .inactive .workspace-label, .inactive .window-controls { opacity:.7; }
  @media (max-width:850px) { .integration-label { display:none; } .workspace-label { margin-inline-start:8px; } }
  @media (max-width:560px) { .workspace-label { display:none; } .drag-region { padding-inline-start:12px; } }
</style>
