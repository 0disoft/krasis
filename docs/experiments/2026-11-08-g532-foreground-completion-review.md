# g532 foreground completion and fallback attribution

## Evidence and correction

The paired g531 trial has twelve matching complete entries, all with fallback.
Its worst A capture is 72.027ms: matching worker activation is 59.268ms, first
foreground request 4.275ms and subsequent verification 0.508ms. Fallback is in
the 10–<100ms bucket. The old report cannot distinguish API rejection from
pending activation or identify an exact fallback substage; it does not prove
that all twelve fallbacks have the same cause.

Windows documents that cross-input-queue SetForegroundWindow returns before the
target processes its internal activation notification. The g531 owner only
peeked QS_SENDMESSAGE and could also consume a successful reply before servicing
the pending notification. This can incorrectly send an accepted request through
the synchronous attach fallback.

g532 peeks with PM_NOREMOVE and no category filter, then services pending
activation after a successful reply, even if that reply arrives before the
native wait. No returned posted input, command or WM_QUIT is removed or
dispatched. Expiry, cancellation and terminal checks precede final service;
native ownership is still verified before capture/ACK. There is no sleep,
activation retry, new thread, process or peer exchange. Existing rejection,
attachment, detach and rollback remain mandatory where required.

Four new native timing buckets distinguish rejection from accepted-but-unverified
fallback and transition recovery from thread attachment. They use 63 of the
existing 64 diagnostic bits in the same 64-byte activation response. Legacy
native indices remain intact; agent IPC capture-array indices move by four in
g532. Historical diagnostic arrays require their generation's original map.
Matching agent/service generations remain required.

References: [asynchronous foreground notification](https://devblogs.microsoft.com/oldnewthing/20161118-00/?p=94745/),
[PeekMessage processing](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-peekmessagew),
[foreground activation wakes message waits](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-msgwaitformultipleobjectsex).

## Validation and limits

- Focused foreground/capture checks passed; full uncached wininput, inputworker
  and handoff tests passed with the selected runtime overlay.
- Full uncached wininput race/checkptr passed (6.592s). Relevant vet passed with
  the existing native `-unsafeptr=false` policy; no analyzer policy changed.
- Deterministic regressions cover the internal notification filter, successful
  reply beating message service, cancellation, exact expiry, terminal state,
  cancellation during service, native ownership, fallback attribution and ACK
  size/reserved-bit validation. Existing handle lifetime checks still pass.
- Opt-in native SourceHook smoke passed with verified ownership and retirement.
  Its first accepted-but-pending request completed without fallback. Four later
  requests were actually rejected: the new counters identify attachment, not
  transition recovery, as the slow interval. Local activation maxima varied up
  to 254.367ms with a 206.876ms attached foreground call. This is not evidence
  that every long Windows activation has been fixed or that device latency fell.
- Early standalone native checks while the installed sharing session was still
  active failed on foreground/attachment access denial. After collecting paired
  diagnostics and disconnecting that session, baseline and final smoke passed.
  Logs remain locally preserved; the failed checks were not hidden or weakened.

Frontend tests/build and package integrity are checked by the normal workspace
builder. No remote CI or push is requested. A new paired physical crossing trial
is required to compare g531's 72.027ms maximum and 12/12 fallback rate. Do not
equate installation readiness with a completed performance trial.

The runtime-flow architecture document is updated. No public command, DB,
dependency, repository hygiene or runner change. Rollback uses the retained
verified g531 setup/update bundle and the ordinary service retirement flow;
never overwrite protected files or reuse unjoined native owners.
