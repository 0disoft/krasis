# g528: Give authenticated source IPC readers dedicated input threads

The g527 confirmed trial has 13 complete pairs. Request submission takes at most
0.012ms and write completion at most 0.020ms, but command receive reaches 46.740ms.
Reply delivery separately reaches 58.697ms. A nearby minute has 108.473ms timer
lateness and 674.367ms sampler lateness. These intervals include reader execution
resumption; they do not isolate Windows kernel transit or prove CPU scheduling
is the sole cause.

Inspection finds that g526's ABOVE_NORMAL (+1) scheduling hint applies only to
low-level-hook, Raw Input and wheel capture owners. The authenticated child command
reader and app ACK/event reader still run on general Go OS threads at NORMAL.
g528 starts those two existing goroutines on locked OS threads, reusing the same
bounded priority policy. Preserve explicitly lower/higher priority, unknown
priority and native hint failure. A hint failure is never a service/recovery error.
No process-wide priority, affinity, real-time priority, busy polling or new worker
per packet/crossing. There is at most one reader thread at each end of a live
source stream. Each restores its original priority before unlocking for Go pool reuse. If native
restoration fails, the thread stays locked through goroutine exit instead of
leaking elevated priority into unrelated Go work.

Authentication before reader startup, per-packet binding/sequence checks,
maintenance ordering, cancellation, native overlapped completion, reply routing,
pressed-state recovery and terminal joins remain in their original order.
The callback body and result propagation are unchanged. No timeout, stream format,
extra ACK or diagnostic JSON change. Existing g527 request partition remains the
live measurement for comparison.

Windows API tests observe +1 reader priority, normal and early-return completion,
and restored native thread priority after callback completion. Restore-failure
policy tests require retaining the OS-thread lock; a failed hint remains benign.
The existing priority-policy cases cover explicit priorities and native failures.
This is a bounded scheduling mitigation, not proof that the 46.740ms/58.697ms tails
are eliminated. Go stop-the-world work, unavailable runtime processors and native
window or external application waits may remain. Rollback is the g527 installer.

Validation: full wininput and inputworker package tests pass, including existing
source authentication, framing, heartbeat, cancellation and cleanup coverage.
Selected source reader/activation/foreground/heartbeat tests also pass with race
instrumentation. Windows priority tests cover normal and early callback return,
original-priority restoration and the keep-locked fallback when restore fails.
Native thread termination is not a timely join signal under race instrumentation;
verification instead uses the restored priority after callback completion.
