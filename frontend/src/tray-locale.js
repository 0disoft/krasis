// Serialize native label updates so a slow older request cannot overwrite the
// user's latest selection. Only the existing supported locale is sent.
export function createTrayLocaleSync(setLocale, reportFailure) {
  let desired;
  let active = false;
  return async locale => {
    if (!['ko', 'en'].includes(locale)) { reportFailure(true); return; }
    desired = locale;
    if (active) return;
    active = true;
    try {
      while (desired) {
        const next = desired;
        desired = undefined;
        let ok = false;
        try { ok = await setLocale(next) === true; } catch {}
        if (!desired) reportFailure(!ok);
      }
    } finally { active = false; }
  };
}
