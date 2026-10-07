const schema = 'krasis.diagnostics/v1';
const codes = new Set(['ok', 'busy', 'cancelled', 'unavailable', 'invalid_peer', 'stale_peer',
  'save_failed', 'send_failed', 'rejected', 'unsupported', 'queued', 'outcome_unknown', 'open_failed',
  'saving', 'pending', 'collected', 'partial', 'local_only', 'storage_limit', 'state_save_failed']);
const transfers = new Set(['NONE', 'QUEUED', 'SENT', 'EXPIRED', 'UNSUPPORTED', 'REJECTED', 'UNKNOWN']);
const collectionStates = new Set(['SAVING', 'PENDING', 'RECEIVED', 'LOCAL_ONLY', 'EXPIRED', 'FAILED', 'UNKNOWN']);
const failure = code => ({ schema, code, saved: false, received: false, transfer: 'UNKNOWN' });

// Collection and folder access have no caller arguments. Native code chooses
// the active peer and retains the request ID; file paths never enter the UI.
export function createDiagnosticsBridge(binding) {
  let busy = false;
  async function call(method, ...args) {
    if (busy) return failure('busy');
    busy = true;
    try {
      const value = await binding[method](...args);
      if (!value || value.schema !== schema || !codes.has(value.code) ||
          typeof value.saved !== 'boolean' || typeof value.received !== 'boolean' ||
          !transfers.has(value.transfer)) return failure('unavailable');
      let collection;
      if (value.collection !== undefined) {
        const c = value.collection;
        if (!c || !collectionStates.has(c.state) || typeof c.localSaved !== 'boolean' || typeof c.peerSaved !== 'boolean' ||
            c.localSaved !== value.saved || c.peerSaved !== value.received ||
            (c.peerSaved && (!c.localSaved || c.state !== 'RECEIVED')) ||
            (c.state === 'RECEIVED' && (!c.localSaved || !c.peerSaved)) ||
            (['PENDING', 'LOCAL_ONLY'].includes(c.state) && (!c.localSaved || c.peerSaved))) return failure('unavailable');
        collection = { state: c.state, localSaved: c.localSaved, peerSaved: c.peerSaved };
      }
      if ((method === 'DiagnosticsCollect' && !collection) ||
          (value.code === 'collected' && collection?.state !== 'RECEIVED') ||
          (value.code === 'pending' && collection?.state !== 'PENDING') ||
          (value.code === 'saving' && (collection?.state !== 'SAVING' || value.saved || value.received)) ||
          (value.code === 'local_only' && collection?.state !== 'LOCAL_ONLY') ||
          (value.code === 'partial' && (!collection?.localSaved || !['EXPIRED', 'FAILED', 'UNKNOWN'].includes(collection.state)))) return failure('unavailable');
      return { schema, code: value.code, saved: value.saved, received: value.received, transfer: value.transfer,
        ...(collection ? { collection } : {}) };
    } catch { return failure('unavailable'); }
    finally { busy = false; }
  }
  return Object.freeze({
    status: () => call('DiagnosticsStatus'),
    collect: () => call('DiagnosticsCollect'),
    export: peer => typeof peer === 'string' && (peer === '' || /^[a-f0-9]{64}$/i.test(peer))
      ? call('DiagnosticsExport', peer) : Promise.resolve(failure('invalid_peer')),
    open: () => call('DiagnosticsOpen'),
  });
}
