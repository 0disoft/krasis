const schema = 'krasis.ui-replacement/v1';
const states = new Set(['IDLE','PREPARING','PENDING','APPLYING','APPLIED','FAILED']);
export const emptyUIReplacement = () => ({schema,state:'UNAVAILABLE',generation:0,canRetry:false});
function narrow(value) {
  if (!value || value.schema !== schema || !states.has(value.state) ||
      !Number.isInteger(value.generation) || value.generation < 0 || value.generation > 1000000 ||
      typeof value.canRetry !== 'boolean' || (value.state === 'IDLE' ? value.generation !== 0 : value.generation === 0) ||
      (value.canRetry && value.state !== 'FAILED')) return emptyUIReplacement();
  return {schema,state:value.state,generation:value.generation,canRetry:value.canRetry};
}
export function createUIReplacementBridge(binding) {
  let cached = emptyUIReplacement(), epoch = 0, reading = false, retrying = false;
  return {
    async status() {
      if (reading || retrying) return cached;
      reading = true;
      const started = epoch;
      try {
        const next = narrow(await binding.UIReplacementStatus());
        if (started === epoch) cached = next;
      } catch { if (started === epoch) cached = emptyUIReplacement(); }
      finally { reading = false; }
      return cached;
    },
    async retry() {
      if (retrying || !cached.canRetry) return cached;
      retrying = true;
      ++epoch;
      try { cached = narrow(await binding.RetryUIReplacement()); }
      catch { cached = emptyUIReplacement(); }
      finally { retrying = false; }
      return cached;
    }
  };
}
