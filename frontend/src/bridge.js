const states = new Set(['LOCAL', 'CONNECTING', 'CONNECTED', 'RECONNECTING', 'DISCONNECTING', 'FAULTED', 'UNKNOWN']);
const inputSetupStates = new Set(['NOT_INSTALLED','UPDATE_REQUIRED','READY','UPDATING','CANCELLED','FAILED','PENDING','UNKNOWN']);

// Only a native-owned binding is accepted. This module never discovers endpoint
// files, makes HTTP calls, stores tokens, or silently falls back to mock success.
export function createBridge(binding) {
  if (!binding || ['Refresh', 'Connect', 'Disconnect'].some(name => typeof binding[name] !== 'function')) {
    throw new Error('bridge_unavailable');
  }
  let busy = false;
  let latest;
  const copy = value => ({ ...value, peers: value.peers.map(peer => ({ ...peer })) });
  async function call(method, args = []) {
    if (busy) throw new Error('bridge_busy');
    busy = true;
    try {
      const value = await binding[method](...args);
      if (!value || value.schema !== 'krasis.desktop/v1' || !states.has(value.state) ||
          !Array.isArray(value.peers) || typeof value.code !== 'string' ||
          typeof value.peerAlias !== 'string' || typeof value.notice !== 'string' ||
          (value.revision !== undefined && (!Number.isSafeInteger(value.revision) || value.revision < 0)) ||
          value.peers.some(p => !p || typeof p.alias !== 'string' || !p.alias ||
            typeof p.fingerprint !== 'string' || !/^[a-f0-9]{64}$/i.test(p.fingerprint))) {
        throw new Error('bridge_protocol');
      }
      if (latest && value.revision > 0 && value.revision < latest.revision) return copy(latest);
      const result = { schema: value.schema, state: value.state, peerAlias: value.peerAlias,
        code: value.code, notice: value.notice,
		resumeState: ['SAVED','FAILED'].includes(value.resumeState) ? value.resumeState : 'UNKNOWN',
        boundaryBlocked: value.state === 'CONNECTED' && ['LOCAL_MENU','MOUSE_CAPTURE','PRESSED_INPUT','MOVE_SIZE'].includes(value.boundaryBlocked) ? value.boundaryBlocked : '',
		inputBackend: ['PROTECTED','PORTABLE'].includes(value.inputBackend) ? value.inputBackend : 'UNKNOWN',
        inputSetup: inputSetupStates.has(value.inputSetup) ? value.inputSetup : 'UNKNOWN',
        ...(value.revision > 0 ? { revision: value.revision } : {}),
        peers: value.peers.map(p => ({ alias: p.alias, fingerprint: p.fingerprint })) };
      if (result.revision > 0) latest = copy(result);
      return result;
    } catch (error) {
      // Native errors can contain transport details. Keep those out of the UI.
      if (error?.message === 'bridge_protocol') throw error;
      throw new Error('bridge_failed');
    } finally { busy = false; }
  }
  return Object.freeze({
    refresh: () => call('Refresh'),
    connect: peer => {
      if (!peer || typeof peer.alias !== 'string' || !peer.alias || peer.alias.length > 256 ||
          typeof peer.fingerprint !== 'string' || !/^[a-f0-9]{64}$/i.test(peer.fingerprint)) {
        return Promise.reject(new Error('bridge_invalid_peer'));
      }
      return call('Connect', [peer.alias, peer.fingerprint]);
    },
    disconnect: () => call('Disconnect'),
    restart: () => call('Restart'),
    inputRefresh: () => call('InputRefresh'),
  });
}
