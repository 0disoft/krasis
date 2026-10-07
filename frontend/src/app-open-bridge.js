const schema = 'krasis.app-open/v1';
const codes = new Set(['pending','launch_accepted','launch_requested','window_ready','window_unconfirmed','launch_failed','launch_record_missing','launch_record_invalid',
  'expired','peer_unresponsive','unknown_request','rejected','unsupported','busy','query_busy','invalid_peer','stale_peer','unavailable','outcome_unknown']);

export function appOpenProgress(previous, status, expired) {
  const transient = ['query_busy','unavailable'].includes(status.code);
  const result = transient ? previous : status;
  const waiting = transient || ['pending','launch_accepted','launch_requested'].includes(status.code);
  if (waiting && expired) {
    return {result: {...result, code: ['launch_accepted','launch_requested'].includes(result.code) ? 'window_unconfirmed' : 'outcome_unknown'}, waiting: false};
  }
  return {result, waiting};
}
export function createAppOpenBridge(binding, storage, clock = {wall:()=>Date.now(), elapsed:()=>performance.now()}) {
  if(!storage){try{storage=globalThis.sessionStorage;}catch{}}
  const key=peer=>'krasis.app-open.'+peer;
  const budget = 70000;
  const records = new Map();
  const waiting = code => ['pending','launch_accepted','launch_requested'].includes(code);
  function remember(v, starting){
    try{
      const saved = resume(v.peer);
      if(waiting(v.code)){
        if(starting && !saved){
          const expires=clock.wall()+budget;
          records.set(v.peer,{id:v.id,expires,deadline:clock.elapsed()+budget,unconfirmed:false});
          storage?.setItem(key(v.peer),JSON.stringify({id:v.id,expires}));
        }else if(saved?.id===v.id && records.get(v.peer)?.unconfirmed){
          // A reloaded receipt gets one bounded observation window only after
          // native code confirms that the original request is still pending.
          const record=records.get(v.peer);
          record.deadline=clock.elapsed()+budget;
          record.unconfirmed=false;
        }
      }else if(!['query_busy','unavailable'].includes(v.code) && saved?.id === v.id){
        records.delete(v.peer);
        const persisted=JSON.parse(storage?.getItem(key(v.peer))??'null');
        if(persisted?.id===v.id)storage?.removeItem(key(v.peer));
      }
    }catch{}
  }
  function resume(peer){
    try{
      if(!/^[a-f0-9]{64}$/i.test(peer??''))return;
      let r=records.get(peer);
      if(!r){
        const saved=JSON.parse(storage?.getItem(key(peer))??'null');
        if(!saved||!/^[a-f0-9]{32}$/i.test(saved.id)||!Number.isSafeInteger(saved.expires)||saved.expires<=0){storage?.removeItem(key(peer));return;}
        r={...saved,deadline:clock.elapsed()+Math.max(0,Math.min(budget,saved.expires-clock.wall())),unconfirmed:true};
        records.set(peer,r);
      }
      return {schema,peer,id:r.id,code:'pending',expires:r.expires,remaining:Math.max(0,r.deadline-clock.elapsed())};
    }catch{}
  }
  const busyPeers = new Set();
  async function call(peer, id) {
    const failure = code => ({schema,peer,id,code});
    if (!/^[a-f0-9]{64}$/i.test(peer ?? '') || typeof peer !== 'string' ||
        typeof id !== 'string' || (id !== '' && !/^[a-f0-9]{32}$/i.test(id))) return failure('invalid_peer');
    if (busyPeers.has(peer)) return failure('query_busy');
    busyPeers.add(peer);
    try {
      const v = await binding.PeerAppOpen(peer, id);
      if (!v || v.schema !== schema || v.peer !== peer || !codes.has(v.code) || typeof v.id !== 'string' ||
          (id && v.id !== id) || (v.id !== '' && !/^[a-f0-9]{32}$/i.test(v.id)) ||
          (v.code === 'pending' && !v.id) || (!id && ['launch_accepted','launch_requested','window_ready','window_unconfirmed'].includes(v.code))) {
        return failure(id ? 'unavailable' : 'outcome_unknown');
      }
      const result={schema,peer,code:v.code,id:v.id};
      remember(result, id === '');
      return result;
    } catch { return failure(id ? 'unavailable' : 'outcome_unknown'); }
    finally { busyPeers.delete(peer); }
  }
  return Object.freeze({resume,expired:(peer,id)=>{
    const pending=resume(peer);
    return !pending||pending.id!==id||pending.remaining===0;
  },open: async peer => {
    const pending = resume(peer);
    if(!pending)return call(peer,'');
    if(!records.get(peer).unconfirmed && pending.remaining>0)return pending;
    const status=await call(peer,pending.id);
    // A local timestamp is not proof that native work ended. Only an explicit
    // new click and a native terminal receipt may start a replacement request.
    return ['expired','unknown_request'].includes(status.code) ? call(peer,'') : status;
  }, status: (peer,id) => {
    if (!id) return Promise.resolve({schema,peer,id:'',code:'invalid_peer'});
    return call(peer,id);
  }});
}
