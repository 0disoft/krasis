# g525 source activation IPC partition

The g524 attended trial contains seven new complete paired leases. Its largest
A capture activation is 211.696ms, including a 209.692ms source IPC response
wait and a 2.003ms command write. The matching owner spans report 4.469ms
queue-to-handler and 29.951ms foreground request. These are nested observations,
not independent values that can be added/subtracted to explain the entire wait.
B preparation peaks at 27.042ms and input dispatch at 21.332ms. TCP RTT is
approximately 4.1-4.4ms without retransmission in the largest activation sample.
The evidence is in `2026-11-04-g524-paired-crossing-review.json`.

g525 records one same-PC nine-point clock chain as eight adjacent microsecond
spans under `handoff.capture.ipc`:

| Field | Boundaries |
| --- | --- |
| requestDeliveryMicros | App command-write entry -> worker decoded command |
| readAuthorizationMicros | Worker decoded command -> live read-owner check return |
| dispatchMicros | Read-owner check return -> serial dispatcher activation entry |
| activationMicros | Dispatcher activation entry -> desktop/native operation return |
| replyAdmissionMicros | Operation return -> reply authority/gate admission |
| replyEncodeMicros | Reply gate admission -> encoded-header write entry |
| replyDeliveryMicros | Write entry -> app decoded reply receipt |
| callerResumeMicros | App decoded reply receipt -> waiting caller resumed |

`replyDeliveryMicros` includes native write, framing, IPC and app-reader
scheduling. It cannot isolate the kernel write completion timestamp. An extra
post-write ACK would add I/O and could itself queue, so no such ACK is added.
The existing native capture buckets/exact spans still refine `activationMicros`.
An entirely valid chain may have measured zero-duration spans. Missing,
nonmonotonic or >120-second chains are not recorded. Metadata is attached only
after matching the exact operation and reply sequence and is frozen at Finish.

The only scheduling change is a one-slot completion channel: the dispatcher no
longer has to wait for the reader to be scheduled merely to publish completion.
The reader's existing command-completion wait still prevents read-ahead of a
subsequent command. This removes one possible blocking rendezvous per completed
native command; no millisecond improvement is claimed without a new trial.

Only activation ACKs grow from 64 to 112 bytes, in one pipe write with no extra
RPC/roundtrip. Normal pointer/input packets carry a nil diagnostic pointer and
do not allocate timestamp arrays. Input authority, pinned-owner checks, command
ordering, time limits, generation checks and uncertain-write fencing are retained.

Validation covers legacy/extended consecutive frame decoding, every truncated
suffix length, invalid optional clocks, measured zero, time-window boundaries,
matching/frozen profiles, one-write ACK delivery, ordered completion and cancel.
The paired installation receipt records exact generation/revision and connection
state separately. A fresh attended trial is needed to identify the dominant new
span and establish whether the 212ms tail improved under comparable CPU load.

No public API, peer input/control protocol, DB, dependency or runner changes.
The private same-version source IPC extension and optional capture JSON change
are documented above. No push or remote CI is part of this request.
