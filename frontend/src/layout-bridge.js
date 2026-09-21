const fp = /^[a-f0-9]{64}$/i;
const edges = new Set(['LEFT', 'RIGHT']);
const rearms = new Set([32, 64, 128]);

function empty(code) {
  return {edge: 'RIGHT', rearm: 32, code};
}

export function createLayoutBridge(binding) {
  if (!binding || ['LayoutStatus', 'LayoutSet', 'LayoutRearm'].some(name => typeof binding[name] !== 'function')) {
    throw new Error('bridge_unavailable');
  }
  async function call(method, args) {
    try {
      const value = await binding[method](...args);
      if (!value || typeof value.code !== 'string') return empty('unavailable');
      const edge = edges.has(value.edge) ? value.edge : 'RIGHT';
      const rearm = rearms.has(value.rearm) ? value.rearm : 32;
      return {edge, rearm, code: value.code};
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
