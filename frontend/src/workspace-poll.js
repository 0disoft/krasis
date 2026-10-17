// Cached workspace reads never occupy the mutation queue. An action invalidates
// both successful and failed reads started before it, including unversioned errors.
export function createWorkspacePoll({ read, ready, apply, failed }) {
  let pending = false, epoch = 0;
  return Object.freeze({
    invalidate() { ++epoch; },
    async refresh() {
      if (pending || !ready()) return;
      const started = epoch;
      pending = true;
      try {
        const value = await read();
        if (started === epoch && ready()) apply(value);
      } catch {
        if (started === epoch && ready()) failed();
      } finally { pending = false; }
    },
  });
}

// Each lane retains its own action gate. A failed lane must not stop unrelated
// reads or break the next polling cycle; the caller waits for all lanes to settle.
export function pollStatus(tasks) {
  return Promise.allSettled(tasks.map(async task => task()));
}
