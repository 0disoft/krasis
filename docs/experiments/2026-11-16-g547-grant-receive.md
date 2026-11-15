# g547 grant receive / execution-resume investigation

## Baseline and limits

The g546 collection `831297fcefb3fb7a7b0358bb6f84f155`, exchange
`a217e4a10a2f49d0ab2f873c3c65ceebe0eb3b8142c02e836d426d848837e52d`,
retains A request-to-grant 325.072 ms, B authorization 30.220 ms and B
request-read-to-grant-written 36.302 ms. A request write is 0.552 ms;
control queue is zero. A TCP RTT estimates are 18.105 / 71.838 ms, with
zero retransmission and timeout counter increments. TCP query durations round
to zero microseconds. These are not a packet timeline, and A/B clocks must
not be subtracted. B processing alone cannot explain A's wait.

A's crossing-period main timer lateness is 51.655 ms and sampler lateness
421.302 ms (B: 6.658 ms). This supports investigating A scheduling but does
not prove that all remaining grant wait is local scheduling. The old
ReadWaiting phase combines peer work, transport, TLS and Go resumption.

## Bounded local observations

The source attempt now samples the read-only Windows socket queue:

* Winsock FIONREAD: unread encrypted socket bytes. Positive means local data
  is queued, not necessarily the matching grant. Zero does not establish
  network delay: TLS and a pending overlapped receive can already own bytes.

A native loopback experiment rejected GetThreadIOPendingFlag as a classifier.
The real Go overlapped TCP read was blocked with no incoming bytes, yet all
three samples reported no pending thread I/O. That signal would incorrectly
classify network waiting as local execution. The candidate implementation and
its misleading exported fields were removed before committing.

Reference: [Winsock IOCTLs](https://learn.microsoft.com/en-us/windows/win32/winsock/winsock-ioctls),
[GetThreadIOPendingFlag](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getthreadiopendingflag).

Only pending retained source attempts poll: 10 ms interval, at most 64 socket
queries per attempt. Idle sessions perform zero queries.
Unsupported socket querying stops the connection's probe. Raw observations
stay process-local in fixed storage; export retains only bounded counts and
durations. Query duration and maximum sample gap expose probe scheduling
limitations. These samples are not percentages of time or measured physical
input-to-display latency. Queries do not consume bytes, send packets or
change socket state. No collector lock spans a native query.

One joined observer belongs to the connection. Shutdown cancels its timer
and joins any in-flight query. Samples for a retired attempt cannot update a
new epoch; observations beyond the grant endpoint are excluded. No thread ID,
handle, address, raw packet or key material is exported.

## Scheduling change

The existing authenticated FIFO wire reader now runs on one locked Windows
thread, with a best-effort NORMAL to ABOVE_NORMAL (+1) priority scope. This
addresses competition with ordinary-priority work during receiving/resuming;
it is not a guaranteed latency bound or proof of a network fix. Explicitly
lower/higher priority is preserved. Restore before returning the thread to
Go; failed restoration leaves it locked until goroutine exit, retiring it.
The reader is joined after priority restoration.

No extra reader, per-frame goroutine, packet reordering, read deadline,
authority bypass, timeout relaxation, heartbeat policy or recovery change is
introduced. Non-Windows reader scheduling remains unchanged; unsupported
socket evidence remains unavailable. Additive local diagnostic fields only:
no wire protocol, business API, DB, dependency, or CI runner changes.

## Validation and rollout

Full handoff, peer, transport, agent, residentagent and productlog tests and
vet (the existing unsafeptr exclusion) passed using the repository runtime
overlay and bounded parallelism. Focused tests cover idle/bounded sampling,
cancel/join, stale epochs, grant endpoints, unsupported
queries, numeric export, optional wrapper forwarding, real loopback FIONREAD,
and real Windows priority restoration.
Focused Windows race tests passed for all six affected packages using the
existing portable LLVM/MinGW toolchain. No race was reported.

An initial highly parallel run without the packaging runtime overlay hit
existing 2-second TLS/guard deadlines during compilation. Revalidation with
the actual runtime overlay and serial package testing passed those tests;
no time limit was enlarged. A transient vet import error occurred while a
previous test invocation overlapped source editing; the final settled-source
test/vet pass succeeded.

Packaging, paired receipt, post-install health and physical crossing results
are recorded separately after rollout. No g547 physical latency improvement
is claimed from a unit test, socket snapshot, or the old g546 sample.

Linux amd64 agent/CLI cross-build passed; native Linux and remote CI were not
run for this Windows-specific change. Socket-queue positives are useful local
backlog evidence, but zero samples still leave network versus IOCP/Go wakeup
unresolved. Existing opt-in Go execution tracing (wire_read regions and
Waiting-to-Runnable-to-Running transitions) is required for exact Go resume
time; it must not be presented as a hardware packet timestamp.
