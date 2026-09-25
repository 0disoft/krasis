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
