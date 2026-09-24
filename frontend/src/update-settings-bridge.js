const schema = 'krasis.update-settings/v1';
const codes = new Set(['ok','permission_saved','peer_status_unavailable','busy','cancelled','unavailable','invalid_peer','stale_peer','rejected','outcome_unknown','agent_changed','unsupported']);
const permissions = new Set(['UNKNOWN','ALLOWED','DENIED']);
const states = new Set(['UNKNOWN','ONLINE','STALE']);
const comparisons = new Set(['UNKNOWN','SAME_BUILD','LOCAL_NEWER','PEER_NEWER','DIFFERENT_BUILD']);
const validPeer = peer => typeof peer === 'string' && /^[a-f0-9]{64}$/i.test(peer);
const validBuild = build => typeof build === 'string' && (build === '' || build === 'development' || /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/i.test(build));
const validGeneration = n => Number.isInteger(n) && n >= 0 && n <= 1000000;
export const emptyUpdateSettings = (peer = '', code = 'unavailable') => ({schema,code,peer,permission:'UNKNOWN',remotePermission:'UNKNOWN',peerState:'UNKNOWN',localGeneration:0,localBuild:'',peerGeneration:0,peerBuild:'',comparison:'UNKNOWN'});

const transferSchema = 'krasis.update-transfer/v1';
export const transferStates = new Set(['IDLE','BUSY','UNAVAILABLE','INVALID_PEER','STALE_PEER','AGENT_CHANGED','REJECTED','PREPARING','QUEUED','SENDING','VERIFYING','DELIVERED','UPDATED','RESTART_UNCONFIRMED','SAME_VERSION','PEER_NEWER','GENERATION_CONFLICT','PEER_BUSY','PEER_UNREACHABLE','PEER_UNSUPPORTED','PEER_NOT_ALLOWED','INVALID_BUNDLE','BUNDLE_MISSING','TRANSITION_REQUIRED','HASH_MISMATCH','STORAGE_FAILED','TRANSFER_INTERRUPTED','PEER_APPLY_FAILED','EXPIRED','PEER_REJECTED','TRANSFER_UNKNOWN']);
export const emptyUpdateTransfer = (peer = '', state = 'IDLE') => ({schema:transferSchema,peer,state,id:'',bytes:0,total:0,generation:0,build:''});
export const transferActive = state => ['PREPARING','QUEUED','SENDING','VERIFYING','DELIVERED'].includes(state);

export function createUpdateSettingsBridge(binding) {
  let busy = false;
  async function transfer(method,peer) {
    const fallback = state => emptyUpdateTransfer(peer,state);
    if (!validPeer(peer)) return fallback('INVALID_PEER');
    if (busy) return fallback('BUSY');
    busy = true;
    try {
      const v = await binding[method](peer);
      if (!v || v.schema !== transferSchema || v.peer !== peer || !transferStates.has(v.state) ||
          typeof v.id !== 'string' || (v.id !== '' && !/^[a-f0-9]{32}$/i.test(v.id)) ||
          !Number.isSafeInteger(v.bytes) || !Number.isSafeInteger(v.total) || v.bytes < 0 || v.total < 0 || v.bytes > v.total || v.total > 96*1024*1024 ||
          !validGeneration(v.generation) || !validBuild(v.build)) return fallback('TRANSFER_UNKNOWN');
      return Object.fromEntries(Object.keys(emptyUpdateTransfer()).map(key => [key,v[key]]));
    } catch { return fallback(method === 'SendUpdate' ? 'TRANSFER_UNKNOWN' : 'UNAVAILABLE'); }
    finally { busy = false; }
  }
  async function call(method, peer, ...args) {
    if (!validPeer(peer)) return emptyUpdateSettings('', 'invalid_peer');
    if (busy) return emptyUpdateSettings(peer, 'busy');
    busy = true;
    try {
      const v = await binding[method](peer, ...args);
      if (!v || v.schema !== schema || v.peer !== peer || !codes.has(v.code) ||
          !permissions.has(v.permission) || !permissions.has(v.remotePermission) ||
          !states.has(v.peerState) || !comparisons.has(v.comparison) ||
          !validGeneration(v.localGeneration) || !validGeneration(v.peerGeneration) ||
          !validBuild(v.localBuild) || !validBuild(v.peerBuild)) return emptyUpdateSettings(peer);
      const result = Object.fromEntries(Object.keys(emptyUpdateSettings()).map(key => [key, v[key]]));
      if (result.peerState !== 'ONLINE') {
        result.peerGeneration = 0; result.peerBuild = ''; result.remotePermission = 'UNKNOWN'; result.comparison = 'UNKNOWN';
      }
      return result;
    } catch { return emptyUpdateSettings(peer, method === 'UpdatePermission' ? 'outcome_unknown' : 'unavailable'); }
    finally { busy = false; }
  }
  return Object.freeze({
    transfer: peer => transfer('UpdateTransfer',peer),
    send: peer => transfer('SendUpdate',peer),
    status: peer => call('UpdateSettings', peer),
    permit: (peer, allow) => typeof allow === 'boolean' ? call('UpdatePermission', peer, allow) : Promise.resolve(emptyUpdateSettings('', 'invalid_peer')),
  });
}
