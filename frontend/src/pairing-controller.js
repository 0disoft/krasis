// Pairing shares the existing operation gate with its background reads. View
// state is read after each await so a delayed query cannot restore old form data.
export function createPairingController(bridge, operation, {active, read, apply, refresh}) {
  return Object.freeze({
    async run(action, background = false) {
      if (!active()) return;
      const {candidate: joinCandidate, pin: joinPIN} = read();
      return operation(async () => {
        if (!active()) return;
        if (action !== 'status') apply({...read(), notice:''});
        const value = action === 'invite' ? await bridge.invite()
          : action === 'search' ? await bridge.search()
          : action === 'cancel' ? await bridge.cancel()
          : action === 'join' ? await bridge.join(joinCandidate, joinPIN)
          : await bridge.status();
        if (!active()) return;
        const current = read();
        let pairing = current.pairing, notice = current.notice;
        if (value.code !== 'busy') {
          pairing = value.code === 'unavailable' && current.pairing.job && !value.job
            ? {...value, job:current.pairing.job} : value;
          if (['pairing_completed','paired_connect_failed','pairing_ended'].includes(value.code)) notice = value.code;
        }
        const candidate = value.candidates[0] && !value.candidates.some(c => c.id === current.candidate)
          ? value.candidates[0].id : current.candidate;
        apply({pairing, notice, candidate, pin:action === 'join' ? '' : current.pin});
        if (value.code === 'pairing_completed' || value.code === 'paired_connect_failed') await refresh();
      }, background);
    },
  });
}
