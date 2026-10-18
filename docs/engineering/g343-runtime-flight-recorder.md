# g343: capture the runtime timeline of the remaining stalls

Paired g342 request `c2866c5d1336fd960abb7cdd68128128` has no new fault on
either peer. A still has 1,173,307 us timer lateness; B's maximum is 7,438 us.
The histogram aggregation uses the original bucket's upper bound. Its last
bucket cannot establish an exact pause duration or that GC caused all lateness.
The older g342 note's "above one second" wording should be read with that limit.

An agent launched with the exact process environment `KRASIS_RUNTIME_TRACE=1`
starts Go's FlightRecorder. Default, service and peer-update launches stay off.
The normal and WMI launch allowlists forward only this explicit value. Recording
expires after 15 minutes or after its first snapshot. The 10-second/2-MiB window
is a runtime hint, not a hard memory or retained-time guarantee.

The existing sampler signals on >=1-second timer lateness or a session failure.
Only a nonblocking, single-use notification runs in callbacks. Another goroutine
flushes the trace and stops recording. Shutdown waits at most two seconds.
The trace contains runtime scheduling, syscall/GC events and source stacks; no
input, PIN, clipboard, credential, peer or arbitrary error annotations are added.

One `diagnostics/runtime-stall.trace` is atomically replaced only after a complete
snapshot. The file has the existing current-user/SYSTEM diagnostics ACL. A strict
8-MiB writer limit rejects oversize traces, removes partial files and preserves
the previous complete snapshot. Raw traces are never part of JSON support export
or peer transfer. Only fixed enable/save/fail/expiry codes enter ordinary logs.
The trace is local debugging evidence; source stacks can contain build paths.

This is evidence capture, not a claim that the remaining stall is fixed.
Use `go tool trace -d=1` or offline pprof output to compare STW boundaries with
long syscall and runnable intervals. Do not run the interactive trace server.
The environment switch is per launch, so an ordinary relaunch disables it.
