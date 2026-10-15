# g313 maintenance liveness

The g312 paired diagnostic request `c09cbbb6b170a337fa55c57c70cef460`
records three A source-reader timeouts while native owners subsequently joined.
At 2026-10-15T17:26:42.887Z source-hook failure was observed first; trust expiry
was recorded at 17:26:42.961Z. The same process interval contains GC pause and
runnable-delay samples in the >1s bucket. This establishes runtime pressure,
not its OS-level cause or the exact duration of any individual pause. An empty
trust read trace can also result from a canceled check before invocation; it
does not independently prove that the worker was never scheduled.

Two defects were reproduced separately from that observation:

- A validated result finished before the trust deadline but observed after it
  was discarded. The old watchdog could also win select against that queued
  verdict. Consume the exact queued result first, retaining negative verdicts.
  A positive verdict is usable only if it finished before the old deadline and
  is still less than five seconds old. New validity is measured from completion,
  never delivery; no expired/canceled session or late validation is revived.
  Do not launch a new read using a budget that has already expired.
- Source keepalive admission repeated SCM service status and WTS logon RPC,
  despite having no capture/input operation. A slow broker could consume the
  source's independent two-second reader budget before the keepalive was sent.
  Native authorities now have a separate heartbeat-only presence check: the
  verified pipe's kernel server PID, or the pinned agent's live process plus
  active console session. No peer-supplied PID is accepted. Zero/closed proofs
  fail, and authorities without that native check retain full authorization.
  Reader binding, sequence, packet validity, write ordering, ACK and idle limits
  stay unchanged. All admission, capture, cursor, input, boundary forwarding,
  recovery and native desktop guards retain their full SCM/WTS checks.

Deterministic tests delay the trust observer past the old deadline with a
timely success, revocation or stale success queued. Source tests keep presence
valid while full input authorization rejects: heartbeat may complete, activation
and action forwarding must fail. Bad bindings never reach authorization.

No peer wire, database, dependency, runner or ACL change. The same one-second
sent-command bound, two-second source idle limit and five-second trust freshness
remain. g312's actual failures cannot all be attributed to the reproduced
delivery race; actual system-wide pauses still require fail-closed cleanup.
Device observation and paired g313 diagnostics are required before judging the
reader/renewal failures resolved in use.
