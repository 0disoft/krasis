# g418: retain the source presence watcher's authority failure

The g417 installed source worker reported `source-reader-authority` after the
09:16:42Z failure. Its independent presence watcher unconditionally replaced
every failed check with ErrAuthority. This discarded the already-sanitized
process/console/service operation and numeric Win32 result before the worker's
final report. Existing authority-detail tests did not traverse this watcher.

The watcher now uses the existing sourceAuthorityResult projection. Known,
closed operation codes and their own numeric native result survive; arbitrary
callback text/errors are still discarded. The return remains an authority
denial. The change grants no input, retries no failed query and changes no
freshness, read, retirement, cleanup or recovery budget. It uses existing wire
operation values and adds no diagnostic payload, schema or sensitive fields.

The regression exercises the watcher, joined source cleanup and final report
projection for process-exited, console-session and service-pipe failures. It
checks that numeric error 5 stays paired with its operation, unknown private
details are removed, and uncertain cleanup still reports unsafe retirement.
The previous watcher fails the regression; the focused source presence,
authority and reader tests pass after the fix.

The user reports B's explicit agent recovery completed. Peer verification is
still pending: a concurrent paired collection returned outcome_unknown while
A was FAULTED. Local diagnostics separately record trust freshness expiry at
09:27:21.9766116Z and recovery deadline exceeded at 09:27:25.9812165Z. A saved
trace copied as `.cache/analysis/g417-source-heartbeat/trust-expiry.trace` has
file time 09:27:01Z, before that expiry; it must not be described as a capture
of the later expiry. The earlier 09:16:42Z trace remains in `failure.trace`.

This repairs failure attribution, not the underlying recurring session faults.
No issue is closed from the diagnostic fix. No push, comment, remote-ref,
signing, firewall, API/DB or runner change. Further validation/application
results follow below.
