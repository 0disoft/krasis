# g342: bound the crash guard's native process wait

The paired g341 export `bed5f2d74002f032385da8f7e661a68e` has no new session
failure, but A's second one-minute runtime summary still contains two observations
in the final coarse GC-duration bucket and timer lateness of 1,718,129 microseconds.
The upper-bound bucket aggregation does not prove either pause exceeded exactly
one second, or that GC caused all timer lateness. Bounding message
queues in g341 therefore did not remove every source of long runtime stalls.

Each product session starts an independent recovery guard. Its waiter called
`exec.Cmd.Wait` immediately after startup, which enters Go's Windows
`os.Process.wait`: `WaitForSingleObject(handle, INFINITE)`. This is another
long-lived syscall exposed to the race described in Go issue 81759. This is a
source-level candidate, not a captured native stack proving the exact pause.

The Windows guard waiter now uses `Process.WithHandle` to pin the already owned
child handle and waits for at most 50 ms per native call. A timeout means the
same child remains alive, not an error or exit. Only after the handle signals
does `Cmd.Wait` reap it and retain normal exit status, stderr drain and resource
cleanup. No PID lookup, GC disabling, lease extension or input retry is added.
An unexpected native wait failure is preserved and normal reaping remains the
fallback; an unconfirmed child exit never closes the guard's Done channel.

Validation uses a real child blocked on its private stdin, proves repeated
timeouts do not fabricate exit, checks exact handle identity and verifies the
final exit status. Existing parent-death recovery, IPC timeout and guard-close
tests remain applicable. Paired runtime observation must establish whether
long stops remain; lock/PIN input still requires a field test.

The guard's writer-valued `Cmd.Stderr` was another permanently blocked syscall:
Go's Windows `os.Pipe` uses a synchronous anonymous pipe, and os/exec drained it
in a background `ReadFile`. Stderr now uses a random, local-only named pipe with
an explicit current-user/SYSTEM DACL, a non-inheritable overlapped parent reader
and a child writer passed as its standard handle. The parent drains through the
runtime IOCP poller, joins EOF before reporting guard completion, and retains the
existing 4 KiB memory limit. No raw stderr is persisted. Native tests verify
read deadlines (poller support), data preservation, EOF drain and oversized
stderr without deadlocking the child or expanding the retained buffer.
