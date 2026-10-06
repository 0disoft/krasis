<script>
  import { onMount } from 'svelte';
  import { createKeyboardBridge, emptyKeyboard } from './keyboard-bridge.js';
  let { peer = '', locale = 'ko' } = $props();
  let value = $state(emptyKeyboard(''));
  let busy = $state(false);
  let notice = $state('');
  let bridge;
  let disposed = false;
  const ko = $derived(locale === 'ko');
  const modes = {ko:{ON:'켜짐',OFF:'꺼짐',UNKNOWN:'확인 전'},en:{ON:'On',OFF:'Off',UNKNOWN:'Unknown'}};
  const local = $derived(value.local);
  const remote = $derived(value.peer === peer ? value.remote : {caps:'UNKNOWN',num:'UNKNOWN',hangul:'UNKNOWN'});
  const canMatch = $derived(!busy && value.code === 'ok' && peer && remote.caps !== 'UNKNOWN' && remote.num !== 'UNKNOWN');
  async function match() {
    if (!bridge || !canMatch) return;
    const target = peer;
    busy = true;
    const next = await bridge.match(target);
    if (!disposed && target === peer) {
      value = next;
      notice = next.code === 'sent' ? (ko ? '맞추기 요청됨' : 'Match requested') : (ko ? '맞추지 못했습니다. 키를 놓고 다시 눌러 주세요.' : 'Could not match. Release held keys and try again.');
    }
    busy = false;
  }
  $effect(() => { peer; notice = ''; });
  onMount(() => {
    let timer;
    async function poll() {
      if (disposed) return;
      const target = peer;
      if (!busy) {
        const next = await bridge.refresh(target);
        if (!disposed && target === peer) value = next;
      }
      if (!disposed) timer = setTimeout(poll, 2000);
    }
    void (async () => {
      try {
        const { Call } = await import(/* @vite-ignore */ '/wails/runtime.js');
        const name = 'github.com/0disoft/krasis/internal/workspacehost.WorkspaceService.';
        bridge = createKeyboardBridge({KeyboardRefresh:peer=>Call.ByName(name+'KeyboardRefresh',peer),KeyboardMatch:peer=>Call.ByName(name+'KeyboardMatch',peer)});
        if (!disposed) await poll();
      } catch { if (!disposed) value = emptyKeyboard(peer); }
    })();
    return () => { disposed = true; clearTimeout(timer); };
  });
</script>
<details class="keyboard-panel">
  <summary>{ko ? '키보드 상태' : 'Keyboard state'}</summary>
  <table>
    <thead><tr><th></th><th>Caps Lock</th><th>Num Lock</th><th>{ko ? '한/영' : 'Korean IME'}</th></tr></thead>
    <tbody>
      <tr><th>{ko ? '이 PC' : 'This PC'}</th>{#each ['caps','num','hangul'] as key}<td>{modes[ko?'ko':'en'][local[key]]}</td>{/each}</tr>
      <tr><th>{ko ? '상대 PC' : 'Other PC'}</th>{#each ['caps','num','hangul'] as key}<td>{modes[ko?'ko':'en'][remote[key]]}</td>{/each}</tr>
    </tbody>
  </table>
  <p>{ko ? 'Caps·Num 확인과 맞추기는 양쪽 Krasis 창을 전면에 두고 사용하세요. 한/영 상태는 앱마다 다릅니다.' : 'Keep both Krasis windows in the foreground to observe and match Caps/Num. IME mode is specific to each app.'}</p>
  <button disabled={!canMatch} onclick={match}>{ko ? '이 PC의 Caps·Num을 상대와 맞추기' : 'Match this PC’s Caps and Num to the other PC'}</button>
  {#if notice}<p role="status">{notice}</p>{/if}
</details>
<style>
  .keyboard-panel{margin:16px 0;font-size:14px;color:var(--muted,#aaa)}
  summary{cursor:pointer}table{margin:12px 0;border-collapse:collapse}th,td{padding:6px 16px;text-align:left}p{margin:8px 0}
</style>
