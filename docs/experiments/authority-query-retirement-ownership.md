# Retain query ownership through resource retirement (#50)

The bounded query worker cleared its Channel query slot before closing retained
resources. If resource retirement itself waited, a concurrent Close observed no
query and blocked on closeOnce. Its caller could therefore exceed the deadline
despite the native query already being bounded. A controlled regression failed:
the second Close remained blocked for the full 100ms observation window until
the test explicitly released the resource closer.

The last query now retains its ownership marker and global task capacity through
resource closure. Concurrent Close returns unconfirmed cleanup immediately;
only actual completion clears the marker and confirms cleanup. Deadline callers
cancel only their still-owned query under the admission mutex, rather than
starting or joining resource cleanup after their budget has expired. A query
already completed before cancellation has no native work to retire. Receiver
input canceled before submission retains an unfenced available proof for fresh
future checks; it still sends nothing and cannot reuse an old result.

The same query owner now exposes private source full/presence variants that
preserve the existing allowlisted native operation and numeric error projection.
The following source-writer unit uses these variants; no public API is added.

Verification: the new pending-resource regression passes after the fix, together
with native-query budget/capacity/retirement tests and inputworker vet/diff checks.
These controlled resource adapters do not certify physical held-input cleanup.
Direct native callback/worker proof ownership remains part of #50/#59's audit.
No DB, dependency, runner, firewall, signing, issue-comment or remote ref changes.
