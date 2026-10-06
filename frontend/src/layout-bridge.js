const fp = /^[a-f0-9]{64}$/i;
const edges = new Set(['LEFT', 'RIGHT']);
const rearms = new Set([32, 64, 128]);
const codes = new Set(['ok', 'unavailable', 'unconfigured', 'invalid_peer', 'not_local', 'rejected']);

function empty(code) {
  return {code};
}

export function createLayoutBridge(binding) {
  if (!binding || ['LayoutStatus', 'LayoutSet', 'LayoutRearm'].some(name => typeof binding[name] !== 'function')) {
    throw new Error('bridge_unavailable');
  }
  async function call(method, args) {
    try {
      const value = await binding[method](...args);
      if (!value || !codes.has(value.code)) return empty('unavailable');
      if (value.code === 'unconfigured') return {edge:'RIGHT', rearm:32, code:'unconfigured'};
      if (value.code !== 'ok') return empty(value.code);
      if (!edges.has(value.edge) || !rearms.has(value.rearm)) return empty('unavailable');
      return {edge:value.edge, rearm:value.rearm, code:'ok'};
    } catch {
      return empty('unavailable');
    }
  }
  return Object.freeze({
    status: fingerprint => fp.test(fingerprint) ? call('LayoutStatus', [fingerprint]) : Promise.resolve(empty('invalid_peer')),
    set: (fingerprint, edge) => {
      if (!fp.test(fingerprint) || !edges.has(edge)) return Promise.resolve(empty('invalid_peer'));
      return call('LayoutSet', [fingerprint, edge]);
    },
    rearm: (fingerprint, distance) => {
      if (!fp.test(fingerprint) || !rearms.has(distance)) return Promise.resolve(empty('invalid_peer'));
      return call('LayoutRearm', [fingerprint, distance]);
    },
  });
}
