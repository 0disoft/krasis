# g422: distinguish source query ownership and failure reasons

g421 paired collection `03665bb12eceeda393f3d6d7df5d33c6` records an A
source-reader-authority failure while B's source retires cleanly. The source
watcher projects query deadlines, capacity exhaustion and channel shutdown to
the same ErrAuthority as a real native rejection. That projection erases the
evidence needed to select a repair. A subsequently falls back to PORTABLE
after its protected service reaches recovery-required (1066).

Retain closed, numeric report operation codes for source query deadline,
cancellation, capacity and closed/fenced channel, and identify service versus
agent in source read authorization. The watcher retains only these fixed codes
and its original ErrAuthority sentinel classification; arbitrary text/identifiers
are not serialized. Concrete native authority/cleanup failures keep precedence.
Append report operation bytes without changing the wire layout or old codes.
Ordinary input/heartbeat APIs, query slots, deadlines, fencing, handle lifetime,
injection and native retirement semantics remain unchanged.

Deterministic tests exercise a real timed-out query against each pinned owner,
then watcher projection, final classification and the report wire. They also
distinguish capacity from closed state and preserve concrete native rejection.
The first wider suite fails only the three portable-pair recovery fixtures
because A's real PORTABLE session holds their fixed ownership object. Existing
safe-stop succeeds before rerunning unchanged inputs without that owner.
No production fence or test assertion is weakened.

This is a diagnostic repair, not a claim that reconnect is fixed. Paired rollout
must produce a specific query failure before the next behavioral change.
No issue closure, push, comment, remote-ref, API/DB, dependency, runner,
firewall, signing configuration or source-of-truth specification change.

## Paired rollout and remaining failures

After safe-stop, the full inputworker suite passes (31.582s) and logindiag
passes. The workspace builder passes 85 UI tests, native frontend/Windows and
embedded installer builds, archive smoke and eight-file verification. Offline
verification accepts g422 revision `23f29dad705e7e507ddab7764ea288b3e761f9dc`,
28,159,986 bytes. A repair exits 0 with receipt 10:42:53.7940303Z. B job
`973fd393a95ab465d0f558d85f595e1c` reaches UPDATED; its successful receipt is
10:43:57.2698195Z. Both PCs reach READY at the same revision. Hosted CI,
publisher signing and physical acceptance matrices were not run.

Collection `d399c557758109d1ab525d594903c45c` records A's 10:44:27Z
source-write-timeout, with joined native/sender/interrupt but failed retirement
delivery. B's source retires cleanly. Later A reaches worker-report-eof while
the worker is still observed RUNNING; service recovery is required. These are
not successful query/retirement or stable-connection acceptance results.

The verified trace relaunch helper attempted to close A's UI, then failed with
"Krasis main loop unavailable". The UI subsequently exited without the helper
completing its relaunch. The user's report that A's app had completely closed
corresponds to this interrupted diagnostic transition; do not classify that UI
closure as an independently observed spontaneous crash. The agent was still
running in portable mode. Existing safe-stop and a same-g422 repair (exit 0,
receipt 10:48:47.4108202Z) complete; the UI is reopened and local tracing is
confirmed enabled at 10:49:10Z. No forced termination or Security UI input.

Collection `341e4dbeae177af7787fe99582f7e635` then records A's 10:49:55Z
source-agent-query-closed, with joined capture/sender/interrupt and failed
retirement delivery. B again retires cleanly. This confirms the new provenance
survives a real paired failure; a closed query may follow another failure and
does not by itself identify the first trigger. The fresh trace saved at
10:49:56Z is copied only into ignored `.cache/analysis/g422-reconnect/`.

The trace's scheduler profile assigns 645.3ms cumulative scheduling delay to
the source IPC reader path, including 637.6ms under SourceSession.Offer and its
observation/queue sends. These are aggregate stack totals, not maximum pauses
or evidence that one channel send blocked for that duration. Blocking profiles
must be distinguished from scheduler profiles. This evidence narrows the next
inspection to the agent reader/queue/observer path and its scheduling, while
retaining the actual write-timeout and missing-worker-report failures.

No new behavioral implementation was started after the ten-minute cutoff.
The reconnect issue remains open. Preserve first-failure ordering before any
secondary watcher query-closed result; do not widen timeouts or suppress
attempted writes without stronger evidence.

Parsed trace transitions bound observed runnable-to-running gaps to 79.05ms
across all goroutines and 15.66ms across candidates with the source reader's
stack. These are observed trace intervals, not physical latency; they do not
explain a one-second write/heartbeat timeout by themselves. The cumulative
profile totals above must not be presented as one long scheduler stall.
At 10:49:54Z the agent logs an actual heartbeat response timeout and closes
its reader before the worker's 10:49:55Z query-closed report. The latter is
secondary evidence, not proof that a fresh native authority denial triggered
this failure. Next inspect the actual pipe read/write and worker-report owners
around that earlier response deadline, preserving completed I/O and cleanup.

A later doctor reports its UI running and the session CONNECTED, but the
protected service STOPPED. Stable protected operation remains unproven;
do not report the final state as healthy or repeatedly reinstall the same
unchanged version to hide the recurring failure. The restored UI is left open.
