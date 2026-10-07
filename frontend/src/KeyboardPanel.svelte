<script>
  import { onMount } from 'svelte';
  import { createKeyboardBridge, emptyKeyboard, keyboardMatchBlock } from './keyboard-bridge.js';
  let { peer = '', locale = 'ko' } = $props();
  let value = $state(emptyKeyboard(''));
  let busy = $state(false);
  let notice = $state('');
  let bridge;
  let disposed = false;
  const ko = $derived(locale === 'ko');
  const modes = {ko:{ON:'켜짐',OFF:'꺼짐',UNKNOWN:'확인 불가'},en:{ON:'On',OFF:'Off',UNKNOWN:'Unavailable'}};
  const local = $derived(value.local);
  const remote = $derived(value.peer === peer ? value.remote : {caps:'UNKNOWN',num:'UNKNOWN',hangul:'UNKNOWN'});
  const block = $derived(keyboardMatchBlock(value, peer, busy));
  const canMatch = $derived(block === '');
  const explanations = {
    ko: {no_peer:'상대 기기를 선택하세요.',busy:'맞추는 중입니다.',waiting:'키보드 상태를 확인하는 중입니다.',unsupported:'상대 Krasis를 업데이트하세요.',remote_unknown:'상대 PC의 Krasis 창을 열어 주세요. 연결 화면의 앱 열기를 사용할 수 있습니다.',local_unknown:'이 PC의 Krasis 창을 클릭해 전면에 두세요.'},
    en: {no_peer:'Select a device.',busy:'Matching keyboard state.',waiting:'Waiting for keyboard state.',unsupported:'Update Krasis on the other PC.',remote_unknown:'Bring Krasis to the foreground on the other PC using Open other app in the connection toolbar.',local_unknown:'Click this Krasis window to bring it to the foreground.'}
  };
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
  <p id="keyboard-match-help">{block ? explanations[ko?'ko':'en'][block] : (ko ? '맞추기를 누르면 이 PC의 Caps·Num을 상대와 같게 바꿉니다.' : 'Matching changes this PC’s Caps and Num to the other PC’s states.')}</p>
  <p>{ko ? '한/영 상태는 현재 앱 기준이며 맞추기에 포함되지 않습니다.' : 'IME mode belongs to the current app and is not included in matching.'}</p>
  <button disabled={!canMatch} aria-describedby="keyboard-match-help" onclick={match}>{ko ? '이 PC의 Caps·Num을 상대와 맞추기' : 'Match this PC’s Caps and Num to the other PC'}</button>
  {#if notice}<p role="status">{notice}</p>{/if}
</details>
<style>
  .keyboard-panel{margin:16px 0;font-size:14px;color:var(--muted,#aaa)}
  summary{cursor:pointer}table{margin:12px 0;border-collapse:collapse}th,td{padding:6px 16px;text-align:left}p{margin:8px 0}
</style>
