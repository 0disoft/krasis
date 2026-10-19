# g359: Retain the rejected source authority check

## Evidence and scope

The g356 source-hook incident retained `report:failed` but no original failure
operation/native error. G358 preserves the final service-side lease result;
static inspection found an earlier loss boundary as well. `Authority.check`
and `checkHeartbeat` deliberately project every error to `ErrAuthority`.
Agent lifetime callbacks additionally discarded WTS/token and process-wait
API failures before that projection. Adding fields only to the final source
history cannot recover these discarded causes.

This change repairs that evidence path, not a reproduced cause of the stall.
The g357 attended movement question is still pending, and its live baseline is
unchanged. No failure of a particular WTS or SCM call has yet been observed.

## Change

Ordinary authority callers, observed input checks and recovery retain their
sentinel-only result. The same lock-protected check implementations now also
serve private source-worker projections. Those projections preserve only eight
fixed operation codes and a numeric Win32 error; arbitrary callback errors,
raw text, handles, identity, session values and paths are never retained.
Process-wait query failures, process exit, console session changes, WTS token
query failure, token logon query failure and changed logon are distinguished.
Service pipe-owner and service-status checks retain their existing native
cause in the same restricted form. A successful API query followed by a policy
mismatch has no invented native error (`0 / known:false`).

Source capture authorization, presence, action/reply writes and bounded advisory
boundary writes use this projection. Full checks and presence checks retain
their separate authority, order and locking. No caching, retry, timeout change,
new permission, new handle owner or new goroutine is introduced. Ordinary agent
client authority and public errors are unchanged.

The report operation vocabulary is append-only and its fixed wire layout is
unchanged. A source authority operation and its numeric error come from the
same typed failure, even when an unrelated joined error comes first. The source
exit's reader/command/join/retirement fields still identify the containing
lifecycle phase. The new diagnostic wrapper is treated like the prior authority
denial only for command classification after confirmed native retirement;
unjoined native/sender owners and channel-close failures still fence recovery.

## Validation and rollout boundary

Inputworker, logindiag, productlog, localapi, residentagent and wininput tests
pass. `go vet -unsafeptr=false` passes for inputworker, logindiag, productlog and
localapi; unfiltered Win32 uintptr callback vet is not claimed.

Tests cover ordinary denial equality, zero/closed proofs, full versus presence
checks, numeric-known/unknown errors, native invalid-token query, the bound
service checker, action and boundary rejection before sequence/write, actual
report encode/read, source-history recording, support sanitization, and
independent uncertain-cleanup fencing. Unknown labels and private error objects
are discarded. The numeric error cannot be taken from a different joined
failure than the selected operation. No native input is injected by these tests.

Bugfix, desktop-app and error-message-integrity review procedures apply.
Generation 359 is reserved locally because the immutable g358 bundle already
exists. G359 is not built or applied; both live PCs retain g357. The separately
verified g358 candidate remains available and is not overwritten. No public API,
DB, runner, dependency or repository-hygiene change, push, public release, CI,
service restart, installation, reboot or live profiling is included. Long-term
stability and attended movement benefit remain unverified.

## Packaging and rejected candidate

G359 at revision `f00300f2c0c929b2499ed8e76c329c24028bad00` was subsequently
packaged. Its 85 UI tests, native UI build, installer provenance, archive roundtrip
and eight-file/icon verification pass. Offline update verification is valid/ok
at 27894470 bytes. It was not sent or installed.

Pre-rollout review found that the new authority-detail priority can hide a
concurrent concrete cleanup operation. The focused regression test using a Go
overlay of g359's actual report_failure.go fails with an authority code/native
5 instead of hook-release/native 170. The correction is tracked as g360; keep
this immutable g359 package unapplied. No existing device failure is attributed
to the unapplied candidate.

Fresh paired collection `5abfe453601c983f09f5da85fbae67cb` at
2026-10-19 17:14:21-22Z retains READY service/input state and no g357
fault/failed/session-end events. Its runtime windows still contain no counted
hook calls or injections; attended movement evidence remains absent. Local
doctor confirms healthy/CONNECTED/PROTECTED/CONFIGURED on unchanged g357.
