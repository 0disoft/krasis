const schema = 'krasis.app-open/v1';
const codes = new Set(['pending','launch_requested','launch_failed','launch_record_missing','launch_record_invalid',
  'expired','unknown_request','rejected','unsupported','busy','invalid_peer','stale_peer','unavailable','outcome_unknown']);
export function createAppOpenBridge(binding) {
  let busy = false;
  async function call(peer, id) {
    const failure = code => ({schema,peer,id,code});
    if (!/^[a-f0-9]{64}$/i.test(peer ?? '') || typeof peer !== 'string' ||
        typeof id !== 'string' || (id !== '' && !/^[a-f0-9]{32}$/i.test(id))) return failure('invalid_peer');
    if (busy) return failure('busy');
    busy = true;
    try {
      const v = await binding.PeerAppOpen(peer, id);
      if (!v || v.schema !== schema || v.peer !== peer || !codes.has(v.code) || typeof v.id !== 'string' ||
          (id && v.id !== id) || (v.id !== '' && !/^[a-f0-9]{32}$/i.test(v.id)) ||
          (v.code === 'pending' && !v.id) || (!id && v.code === 'launch_requested')) {
        return failure(id ? 'unavailable' : 'outcome_unknown');
      }
      return {schema,peer,code:v.code,id:v.id};
    } catch { return failure(id ? 'unavailable' : 'outcome_unknown'); }
    finally { busy = false; }
  }
  return Object.freeze({open: peer => call(peer, ''), status: (peer,id) => {
    if (!id) return Promise.resolve({schema,peer,id:'',code:'invalid_peer'});
    return call(peer,id);
  }});
}
