const idPattern = /^[a-f0-9]{32}$/;
const pinPattern = /^\d{6}$/;

function empty(code) {
  return {schema: 'krasis.pairing/v1', code, pin: '', job: '', alias: '', candidates: []};
}

export function createPairingBridge(binding) {
  if (!binding || ['PairStatus', 'PairInvite', 'PairSearch', 'PairJoin', 'PairCancel'].some(name => typeof binding[name] !== 'function')) {
    throw new Error('bridge_unavailable');
  }
  async function call(method, args = []) {
    try {
      const value = await binding[method](...args);
      if (!value || value.schema !== 'krasis.pairing/v1' || typeof value.code !== 'string') return empty('unavailable');
      const pin = typeof value.pin === 'string' && pinPattern.test(value.pin) ? value.pin : '';
      const shown = value.code === 'pin_waiting' ? pin : '';
      const job = typeof value.job === 'string' && idPattern.test(value.job) ? value.job : '';
      const alias = typeof value.alias === 'string' && value.alias.length > 0 && value.alias.length <= 64 ? value.alias : '';
      const candidates = Array.isArray(value.candidates) ? value.candidates.filter(c => c && idPattern.test(c.id)).map(c => ({id: c.id})) : [];
      return {schema: value.schema, code: value.code, pin: shown, job, alias, candidates};
    } catch {
      return empty('unavailable');
    }
  }
  return Object.freeze({
    status: () => call('PairStatus'),
    invite: () => call('PairInvite'),
    search: () => call('PairSearch'),
    join: (id, pin) => {
      if (!idPattern.test(id) || !pinPattern.test(pin)) return Promise.resolve(empty('invalid_pin'));
      return call('PairJoin', [id, pin]);
    },
    cancel: () => call('PairCancel'),
  });
}
