// Native layout reads and writes share the workspace gate. Only the selected
// peer may update the view; queued edits recheck connection state before writing.
export function createLayoutController(bridge, operation, { currentPeer, canChange, apply }) {
  function run(method, peer, value) {
    const background = method === 'status';
    if (!peer || currentPeer() !== peer) return Promise.resolve();
    return operation(async () => {
      if (currentPeer() !== peer || (!background && !canChange())) return;
      const next = await bridge[method](peer, value);
      if (currentPeer() === peer) apply(next);
    }, background);
  }
  return Object.freeze({
    status: peer => run('status', peer),
    set: (peer, edge) => run('set', peer, edge),
    rearm: (peer, distance) => run('rearm', peer, distance),
  });
}
