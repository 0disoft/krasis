# g526 bounded dedicated input-thread priority hint

The user-confirmed g525 trial has 15 complete paired leases: A capture median
45.087ms, maximum 808.306ms. The worst matching clock chain assigns 711.814ms
to worker activation, including a measured 260.631ms foreground request and
a native Show bucket of 100..<1000ms. Dispatch, reply admission and app resumption
also grow in that sample. The nearby A OS trail has all 64 CPU windows >=95%
busy, with OS-counter reads up to 533.911ms. B preparation remains <=32.532ms
and input dispatch <=26.928ms. Both services are READY without new recovery or
reconnect faults. See `2026-11-05-g525-confirmed-stutter-review.json`.

The inspected Go 1.27.3 `runtime/os_windows.go` disables process dynamic priority
boosts during osinit because ordinary Go threads mix GUI, I/O and computation.
Krasis source input owners are different: `startSourceInputThread` already pins
one goroutine to each dedicated native input thread until OS-thread retirement.
They currently use relative priority NORMAL (base 8 in a normal process).

g526 applies ABOVE_NORMAL (+1; base 9 in a normal process) only to those three
owners, once before their native startup work. GetThreadPriority failure, an
explicit non-NORMAL priority or SetThreadPriority failure skips the advisory hint
and leaves the original run/result/cleanup intact. No new error or retry policy.
The original outer runtime.LockOSThread remains held at goroutine exit; the
elevated thread cannot be returned to unrelated Go work. Nested locks are balanced
as before. Process priority, dynamic boosts, foreground proof, native suppression,
input replay rules, thread joins and protected-service recovery remain unchanged.

Policy tests cover relative priorities -15, -2, -1, 0, +1, +2, +15, unknown and
native set failure. A real Windows API test creates all three owner variants and
checks +1 priority plus original successful/error completion results. Existing
source activation, shutdown and heartbeat tests continue to cover lifecycle.
Two native calls occur per successful owner startup (six per source instance),
with no extra work per input frame, boundary crossing, IPC ACK or heartbeat.

Windows recommends raising dedicated input-thread priority over background
computation: [Scheduling priorities](https://learn.microsoft.com/en-us/windows/win32/procthread/scheduling-priorities).
This is a scheduling mitigation under the observed contention, not proof that
every foreground/ShowWindow stall is CPU-ready wait. External app message-queue
blocking, DWM and driver waits can remain. The post-install receipt verifies
version/READY/connection; a new attended trial is required for latency claims.

No public API, private/peer wire protocol, JSON schema, DB, dependency or runner
change. No process-level priority, affinity or unrelated-workload change. No push
or remote CI requested. Rollback is the previous g525 installer.
