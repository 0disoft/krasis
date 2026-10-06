// Native layout reads and writes share the workspace gate. Only the selected
// peer may update the view; queued edits recheck connection state before writing.
export function createLayoutController(bridge, operation, { currentPeer, canChange, apply }) {
  let acceptedPeer = '', accepted = {edge:null, rearm:null};
  function run(method, peer, value) {
    const background = method === 'status';
    if (!peer || currentPeer() !== peer) return Promise.resolve();
    return operation(async () => {
      if (currentPeer() !== peer || (!background && !canChange())) return;
      const next = await bridge[method](peer, value);
      if (currentPeer() !== peer) return;
      if (next.code === 'ok' || next.code === 'unconfigured') {
        acceptedPeer = peer;
        accepted = {edge:next.edge, rearm:next.rearm};
      }
      const layout = acceptedPeer === peer ? accepted : {edge:null, rearm:null};
      apply({...layout, code:next.code}, peer);
    }, background);
  }
  return Object.freeze({
    status: peer => run('status', peer),
    set: (peer, edge) => run('set', peer, edge),
    rearm: (peer, distance) => run('rearm', peer, distance),
  });
}
