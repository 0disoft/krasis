# g549 activation reply handoff

## Evidence and attribution

The preceding g548 trial measured 41.354ms from the local source reader's
completed activation reply read to the caller's resumption. This interval also
includes reader validation and publication; it is not exclusively caller
runnable time. Native activation was 14.694ms in that exchange. The distinct
grant exchange had 130.869ms request-written-to-grant-read and 30.521ms receiver
processing, leaving 100.348ms unresolved. Independent PC timestamps are never
subtracted. The original g548 review incorrectly described two zero-input
profiles as `incomplete: true`; their flags are false. They still have no input
samples and must not count as measured ordinary input.

The source reader is locked to a Windows thread. After publishing the activation
reply into its one-slot channel, it immediately starts the next overlapped pipe
read. `WaitForSingleObject` in that read can retain the Go processor until the
runtime retakes it, while the newly runnable caller waits. This is a local
Go/Windows scheduling dependency, independent of peer packet transmission.

A Windows probe with one Go processor, a locked reader, a one-slot reply channel,
and a subsequent 20ms native wait compared the current ordering to an explicit
yield immediately after publication. Each variant ran 24 times in each of two
uncontrolled repetitions, using the repository's Go 1.27.3 runtime overlay:

| Repetition | Existing median / max, ms | Yield median / max, ms |
| --- | ---: | ---: |
| 1 | 6.177 / 31.212 | <0.001 / 3.871 |
| 2 | 5.384 / 31.226 | 1.331 / 23.429 |

These are mechanism-level reply handoff observations, not a 12-processor paired
speedup or a guaranteed latency bound. Windows descheduling and other Go work
remain possible after yielding. The native wait's completion is joined each
iteration; no thread, I/O or handle is abandoned.

Collection `c6b02fbfd15883baba6917df3b4c42b6` retains twelve further g548
source exchanges during the user's trial. Caller-resume reached 27.100ms; a
122.942ms grant had 31.300ms receiver processing, residual 91.642ms. This
confirms that both tails remain intermittent rather than constant.

The bounded opt-in trace also recorded a source-hook timeout on g548. Its final
flight window contains a 6.225s `gc_metrics` region and a 453ms OS-counter region,
but no complete physical handoff. Go's `Running` state does not prove continuous
OS CPU execution. The earlier latency snapshot was overwritten by the terminal
snapshot. Therefore this trace cannot attribute the user's individual grant or
41.354ms sample to that metrics call. Tracing was disabled and normal launch
and connectivity restored after collection.

The short, header-limited packet monitor session ended before the later twelve
crossings. The retained matching traffic is predominantly transmit-side; there
is no complete matching receive/application pair. Its hardware timestamps do
not establish a network/OS split for 100.348ms or 91.642ms. Neither those
residuals nor an empty socket-queue sample is labeled pure network latency.
The capture and its temporary filter were stopped and removed.

## Change

After a validated activation ACK is successfully published, the reader calls
`runtime.Gosched` once before entering its next native read. Full queues still
fail closed, and no yield occurs on failed publication or on ordinary input,
heartbeat, admission, deactivation or stop replies. Authentication, binding and
sequence validation precede publication as before; one reader/writer, FIFO,
deadlines, retirement and exact ACK results remain unchanged.

An additive local `capture.ipc.reply` profile partitions the legacy interval into
`readToPublishMicros` and `publishToResumeMicros`. Their sum must exactly match
`callerResumeMicros`; missing, reversed and out-of-window clocks stay unmeasured.
The publication clock precedes the channel send, so the second span can include
enqueue scheduling and must not be called exact Go runnable time.

Opt-in `krasis.source.reply_wait` regions and fixed decoded
`krasis.peer.control_read` markers permit future traces to locate the waiter and
grant reader, then separate Go Waiting/Runnable/Running/Syscall intervals.
These markers never contain payloads, IDs, keys or addresses. The network marker
is after full TLS frame read, not NIC arrival. Tracing-disabled reads do no
additional control-payload decoding.

## Validation and rollout

Unit tests cover activation-only yield, publication-before-yield, nil/full
queues, exact reply/FIFO preservation, measured zero, window boundaries,
overflow, missing/reversed clocks, consistent sums and trace payload exclusion.
Final test, vet, race, package and paired installation results follow below.

Full tests passed for handoff, inputworker, peer, productlog and residentagent.
After the final control-marker change, focused peer trace/allocation tests
passed again. Vet passed for those five packages with the repository's existing
`-unsafeptr=false` exclusion. Focused Windows race tests passed for handoff,
inputworker and peer using the portable LLVM/MinGW compiler; no race reported.
Commands used the verified Go 1.27.3 compiler, existing runtime overlay and
bounded package/build parallelism. Formatting and `git diff --check` passed.

The second terminal trace retains two grant-handler regions. Its longest is
18.010ms: Go Waiting/select 15.563ms, Runnable 1.619ms, Syscall 0.491ms and
Running 0.337ms. This is a separate local execution sample, not a match for the
historical 41.354ms or 100.348ms exchange. The long wire-read region also includes
idle and teardown; its Waiting/network label cannot be equated to packet transit.

No peer/native wire format, business API, DB, dependencies, repository hygiene
or CI runner policy changes. Diagnostic JSON has optional additive fields.
Linux source-capture scheduling is unchanged. Remote CI and native Linux VM
validation are outside this Windows-specific rollout; no such result is claimed.
