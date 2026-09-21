const schema = 'krasis.diagnostics/v1';
const codes = new Set(['ok', 'busy', 'cancelled', 'unavailable', 'invalid_peer', 'stale_peer',
  'save_failed', 'send_failed', 'rejected', 'unsupported', 'queued', 'outcome_unknown', 'open_failed']);
const transfers = new Set(['NONE', 'QUEUED', 'SENT', 'EXPIRED', 'UNSUPPORTED', 'REJECTED', 'UNKNOWN']);
const failure = code => ({ schema, code, saved: false, received: false, transfer: 'UNKNOWN' });

// Only a peer identity crosses into export. Folder access has no caller argument.
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
      return { schema, code: value.code, saved: value.saved, received: value.received, transfer: value.transfer };
    } catch { return failure('unavailable'); }
    finally { busy = false; }
  }
  return Object.freeze({
    status: () => call('DiagnosticsStatus'),
    export: peer => typeof peer === 'string' && (peer === '' || /^[a-f0-9]{64}$/i.test(peer))
      ? call('DiagnosticsExport', peer) : Promise.resolve(failure('invalid_peer')),
    open: () => call('DiagnosticsOpen'),
  });
}
