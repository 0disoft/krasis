const schema = 'krasis.app-open/v1';
const codes = new Set(['pending','launch_requested','window_ready','window_unconfirmed','launch_failed','launch_record_missing','launch_record_invalid',
  'expired','peer_unresponsive','unknown_request','rejected','unsupported','busy','invalid_peer','stale_peer','unavailable','outcome_unknown']);

export function appOpenProgress(previous, status, expired) {
  const transient = ['busy','unavailable'].includes(status.code);
  const result = transient ? previous : status;
  const waiting = transient || ['pending','launch_requested'].includes(status.code);
  if (waiting && expired) {
    return {result: {...result, code: result.code === 'launch_requested' ? 'window_unconfirmed' : 'outcome_unknown'}, waiting: false};
  }
  return {result, waiting};
}
export function createAppOpenBridge(binding, storage) {
  if(!storage){try{storage=globalThis.sessionStorage;}catch{}}
  const key=peer=>'krasis.app-open.'+peer;
  function remember(v){
    if(!storage)return;
    try{
      if(['pending','launch_requested'].includes(v.code)){
        if(!storage.getItem(key(v.peer)))storage.setItem(key(v.peer),JSON.stringify({id:v.id,expires:Date.now()+70000}));
      }else if(!['busy','unavailable'].includes(v.code))storage.removeItem(key(v.peer));
    }catch{}
  }
  function resume(peer){
    try{
      if(!/^[a-f0-9]{64}$/i.test(peer??''))return;
      const r=JSON.parse(storage?.getItem(key(peer))??'null');
      if(r&&/^[a-f0-9]{32}$/i.test(r.id)&&Number.isSafeInteger(r.expires)&&r.expires>Date.now()&&r.expires<=Date.now()+70000)return {schema,peer,id:r.id,code:'pending',expires:r.expires};
      storage?.removeItem(key(peer));
    }catch{}
  }
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
          (v.code === 'pending' && !v.id) || (!id && ['launch_requested','window_ready','window_unconfirmed'].includes(v.code))) {
        return failure(id ? 'unavailable' : 'outcome_unknown');
      }
      const result={schema,peer,code:v.code,id:v.id};
      remember(result);
      return result;
    } catch { return failure(id ? 'unavailable' : 'outcome_unknown'); }
    finally { busy = false; }
  }
  return Object.freeze({resume,open: peer => call(peer, ''), status: (peer,id) => {
    if (!id) return Promise.resolve({schema,peer,id:'',code:'invalid_peer'});
    return call(peer,id);
  }});
}
