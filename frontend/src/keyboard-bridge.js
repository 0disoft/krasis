const modes = new Set(['ON', 'OFF', 'UNKNOWN']);
const codes = new Set(['ok', 'sent', 'rejected', 'unavailable', 'unsupported']);
export const unknownKeyboard = () => ({caps:'UNKNOWN',num:'UNKNOWN',hangul:'UNKNOWN'});
export const emptyKeyboard = peer => ({schema:'krasis.keyboard-state/v1',peer,code:'unavailable',local:unknownKeyboard(),remote:unknownKeyboard()});
const valid = value => value && ['caps','num','hangul'].every(key => modes.has(value[key]));
const copy = value => ({caps:value.caps,num:value.num,hangul:value.hangul});
export function createKeyboardBridge(binding) {
  let busy = false;
  async function call(method, peer) {
    if (busy || (peer !== '' && !/^[a-f0-9]{64}$/i.test(peer))) return emptyKeyboard(peer);
    busy = true;
    try {
      const value = await binding[method](peer);
      if (value?.schema !== 'krasis.keyboard-state/v1' || value.peer !== peer || !codes.has(value.code) || !valid(value.local) || !valid(value.remote)) return emptyKeyboard(peer);
      return {schema:value.schema,peer,code:value.code,local:copy(value.local),remote:copy(value.remote)};
    } catch { return emptyKeyboard(peer); }
    finally { busy = false; }
  }
  return Object.freeze({refresh:peer=>call('KeyboardRefresh',peer),match:peer=>peer ? call('KeyboardMatch',peer) : Promise.resolve(emptyKeyboard(peer))});
}
