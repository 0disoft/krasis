# g401: retain the outer hook callback's progress (#59)

Two regression tests failed on g400's sourceHookProgress: a nested callback
renewed the outer callback's one-second deadline, and its completion cleared
activity while the outer callback was still running. A test through the actual
timedSourceCallback wrapper reproduced the missing stall diagnosis. No physical
Windows hook removal or historical recovery incident is inferred from this.

Track nesting on the native callback owner and publish the oldest active start
until every nested invocation unwinds. The watchdog still reads atomics only;
callback timing, suppression, desktop authorization and cleanup thresholds are
unchanged. Existing allocation-free observation and idle/raw-progress checks
remain applicable. No new queue, goroutine, mutex, I/O or native query is added.

Runtime hook telemetry had the same defect. Its serialized timing samples now
keep the outer inclusive duration while counting every delivered completion.
The maximum duration includes time in nested callbacks and downstream hook
chains; it does not attribute that time to Krasis. Atomic snapshots remain
approximate. An unmatched completion cannot underflow nesting or corrupt the
following span. This telemetry never grants input authority.

Wininput, agent, inputworker and residentagent tests pass; the new nested-span
telemetry regression also failed before its fix. Relevant vet passes, using
CI's -unsafeptr=false for wininput. Actual nested Windows callbacks, native input
latency and physical fault/lock/DPI acceptance are not established by fixtures.
#59 remains open: producer/physical semantic locks, synchronous desktop/cursor
work and their final owner/liveness acceptance still require work. No public
API, DB, dependency, runner, firewall, signing, issue-comment or remote-ref
changes. Generation 401 identifies the authorized A/B executable update.

## Paired application

Both PCs applied revision 432cbdc6fb86b6334b459746cd3e0019c48432d9, g401.
UI 85 tests, native builds, embedded installer provenance and eight-file bundle
checks pass; offline verification accepted 28,127,078 bytes. A installer exited
0 and the protected installed agent digest matches the bundle. B receipt
54fb8ba07ad48f5b34474d97222fc562 reports UPDATED. A was explicitly reconnected
after B replacement, without another update or a Disconnect command.

Collection 8d5102c16e0f31b61f4201ddfbc0042d captured B still RECONNECTING.
Follow-up 70cc164989f1fbd6b11dbaf9a51b705b at 00:20:04Z/06Z confirms both
CONNECTED, both services READY, successful installer receipts and protected
capture readiness. Its 90 A / 71 B current-build events contain no fault or
startup-failed code. Doctor confirms matching UI/resident/installed builds.
This short observation does not establish real nested-hook, held-input or
performance acceptance. No issue was closed from these observations.
