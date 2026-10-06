// Polls share the native-call gate with user actions, but never change the
// visible busy state. A click during a poll waits instead of being dropped.
export function createOperationGate(setBusy) {
  let pending;
  let acting = false;
  return async function run(operation, background = false) {
    if (acting || (background && pending)) return;
    const previous = pending;
    if (!background) { acting = true; setBusy(true); }
    const current = (async () => {
      if (previous) {
        try { await previous; } catch { /* A failed poll must not discard a click. */ }
      }
      return operation();
    })();
    pending = current;
    try { return await current; }
    finally {
      if (pending === current) pending = undefined;
      if (!background) { acting = false; setBusy(false); }
    }
  };
}
