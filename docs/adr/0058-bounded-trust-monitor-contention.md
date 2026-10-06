# ADR 0058: Bound trust-monitor lock contention

- Status: Accepted

The g30 reports show B stopping first with `fault_session_unknown`, followed by
A seeing EOF. They do not prove whether the monitor, guard, or another component
caused this incident.

Source review of the g30 implementation found that the one-second product trust
monitor cancels the session on any PeerCertificate error. Store reads then took
a nonblocking exclusive file lock, so a concurrent status or management read
could return ErrStoreBusy without any trust change. This was a reproducible
failure path, not evidence of revocation.

Retry only the exact ErrStoreBusy sentinel, at 25 ms intervals, at most five
checks (100 ms total scheduled waiting). No cached trust verdict substitutes for
the successful read. All other failures stop immediately; persistent contention
also stops. Cancellation interrupts waits. Authentication and mutation APIs keep
their existing fail-closed behavior. No lock is removed or bypassed.

Record fixed monitor-stop codes before cancellation, distinguishing exhausted
contention, trust-check failure, guard exit, certificate deadline, and guard-close
failure. Never record error text, peer identifiers, certificate bytes or paths.

Agent generation 31 deploys this change. The native GUI and input protocol are
unchanged. Targeted tests cover bounded retries, revocation after contention,
I/O failure, cancellation, monitor reason ordering, and sanitized export.
Two-device testing is still required to establish whether this fixes the observed
incident; these reports alone cannot establish that causal link.

Subsequent trust-store reads use a shared file lock; mutation and revocation
remain exclusive. Generation 65 adds coarse timing buckets for pre-lock
directory and ACL work, lock acquisition, in-lock store read and ACL work,
certificate validation, and retry waiting. On timeout it also records the
active phase. These fixed codes contain no peer, path, certificate, or error
text. They identify where a future failure spends time; they do not by
themselves prove that the g54 study-pair disconnect is resolved.

Generation 68 keeps those aggregate buckets and additionally records fixed
substeps for parent/directory ACL and metadata checks, directory and lockfile
opens, lockfile ACL, and trust-file open, ACL, byte read, decode, and close. The active
substep is recorded on timeout; completed substeps under 10 ms are omitted.
Substep durations overlap their aggregate
`prelock` or `store_read` duration and must not be added to it. Codes still
contain no path, peer identity, certificate, input, or raw error. This narrows
the next failure investigation; it does not relax the one-second trust deadline
or change fail-closed behavior.

Generation 71 separates a slow trust read from a negative trust verdict. The
g68 two-device logs showed two one-second trust-check deadlines, with different
active steps (certificate and directory ACL); A disconnected and B saw EOF.
The logs do not identify the external source of the delay. A timed-out or
still-running read is inconclusive: keep the pinned session for at most five
seconds since its last complete trust validation and retry on subsequent ticks.
One blocked worker remains the maximum. A hard freshness deadline also stops
the session if ticks cease; a late result cannot revive it. Revocation, missing
trust, corrupt data, other read errors, guard exit and certificate expiry still
stop immediately. This deliberately permits up to five seconds of previously
validated trust during a filesystem stall, rather than treating the one-second
sampling budget as a revocation. Fixed deferred/recovered/expired codes reveal
the outcome without paths or certificate data.

Generation 88 applies the same five-second freshness boundary to an exhausted
nonblocking store-lock retry. A busy lock is an inconclusive read, not a trust
denial: a later successful read recovers without disconnecting, while persistent
contention still cancels the session at the freshness deadline. Negative trust
verdicts still stop immediately. This does not address a read blocked inside
Windows parent-directory ACL validation; that separate delay remains visible in
the fixed trust timing codes and requires two-device follow-up.

Generation 89 keeps the same trust checks and freshness deadline. When a parent
directory security check is still running at a timeout, diagnostics now identify
whether it is traversing reparse components, reading directory metadata,
querying the Windows ACL, or checking that ACL. The existing parent-ACL timing
bucket remains the whole operation; the new fixed codes identify only the
active call and contain no path, SID, certificate, or error text. A later
two-device diagnostic is needed before changing the security check itself.

Generation 194 retains one pending trust read within the existing five-second
freshness deadline. The one-second budget reports a deferred read without
cancelling it. The monitor consumes completion directly, even without another
sampling tick. A successful read must complete and be consumed before the old
deadline; the next deadline starts at completion, not at delayed consumption.
Further ticks do not start competing reads. The process-wide single-worker slot
still bounds a blocked read after cancellation. A negative verdict, guard exit,
certificate expiry or freshness expiry stops the session; a result belonging to
a stopped session cannot refresh or restart it. Lock retries, ACL checks and
certificate validation remain unchanged. This prevents a valid 1-4 second read
from being discarded solely because it exceeded the observation budget.
The g192 paired evidence and remaining uncertainty are recorded in
`docs/experiments/g192-trust-recovery-observations.md`.
