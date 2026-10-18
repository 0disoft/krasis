# g341: bound idle native message waits

The paired g340 export `7bb0e7296f8eb3b71e11699476d73d05` contains A-side
GC stopping pauses exceeding one second and timer lateness of 4,370,796 and
5,596,075 microseconds around trust freshness and session failures. B's matching
records do not contain those long GC stops. This ties the outages to an A-side
runtime stall; it does not identify the exact blocked native call.

Go's [issue 81759](https://github.com/golang/go/issues/81759) and proposed
[fix 81835](https://github.com/golang/go/pull/81835) describe a syscall-P race
that can leave GC waiting for a blocked syscall. The fix remains unmerged at
inspection, and the installed Go 1.27.1 and downloaded Go 1.27.2 runtime sources
both lack the repeated syscall-P sweep. A toolchain upgrade alone is therefore
not used as the remedy.

The lifecycle monitor and three input message pumps previously waited in
`GetMessageW` without a bound. They now share `winmessage.Get`, which removes
the next message with `PeekMessageW` and waits using
`MsgWaitForMultipleObjectsEx` for at most 50 ms per native call. Posted messages
wake immediately; this is not a 50 ms polling delay before dispatch. Existing
message order, owner thread, window dispatch, and WM_QUIT handling are retained.
The cancellable source pump's native fallback also uses the 50 ms bound.

This removes these unbounded syscall waits without disabling GC, extending
trust validity or leases, retrying input, or modifying the Go runtime. It does
not bound time spent inside arbitrary Windows callbacks or other native calls.

Validation covers idle native timeout, already examined queued messages,
ordered thread messages, WM_QUIT, source-loop cancellation and lifecycle owner
cleanup. A separate 20-second forced-GC experiment on Go 1.27.1 did not reproduce
the upstream race in either the old or bounded minimal loop; it is not evidence
that g340 is safe or that this mitigation alone fixes all runtime stalls.
Paired g341 observation and actual transition tests remain necessary.

Rollback uses a previously verified complete generation through the existing
update channel. Keep the failed g340 request exports for comparison.
