const schema = 'krasis.update-settings/v1';
const codes = new Set(['ok','permission_saved','peer_status_unavailable','busy','cancelled','unavailable','invalid_peer','stale_peer','rejected','outcome_unknown','agent_changed','unsupported']);
const permissions = new Set(['UNKNOWN','ALLOWED','DENIED']);
const states = new Set(['UNKNOWN','ONLINE','STALE']);
const comparisons = new Set(['UNKNOWN','SAME_BUILD','LOCAL_NEWER','PEER_NEWER','DIFFERENT_BUILD']);
const validPeer = peer => typeof peer === 'string' && /^[a-f0-9]{64}$/i.test(peer);
const validBuild = build => typeof build === 'string' && (build === '' || build === 'development' || /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/i.test(build));
const validGeneration = n => Number.isInteger(n) && n >= 0 && n <= 1000000;
export const emptyUpdateSettings = (peer = '', code = 'unavailable') => ({schema,code,peer,permission:'UNKNOWN',remotePermission:'UNKNOWN',peerState:'UNKNOWN',localGeneration:0,localBuild:'',peerGeneration:0,peerBuild:'',comparison:'UNKNOWN'});

export function createUpdateSettingsBridge(binding) {
  let busy = false;
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
    status: peer => call('UpdateSettings', peer),
    permit: (peer, allow) => typeof allow === 'boolean' ? call('UpdatePermission', peer, allow) : Promise.resolve(emptyUpdateSettings('', 'invalid_peer')),
  });
}
