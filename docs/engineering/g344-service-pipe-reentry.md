# g344: reduce the initial service-pipe polling gap

Paired g343 request `6dabe3594688d1843e2d6f1b13160a4f`, exchange tag
`7e26a4e3216801bf40a2622dc7560f3a2788532e5a96d149ae71d587add9b26f`,
has a 96,653-us source grant wait and an 88,011-us receiver request-to-grant span.
Receiver authorization takes 83,373 us, including 81,481-us connection setup:
61,451 us opening the pipe, 2,322 us service authentication and 17,707 us readiness.
The prepared worker was used. These are nested local durations, not additive
cross-machine clock offsets. The first g343 field trial has no new fault.

The dial loop previously slept 20 ms after every transient busy/missing open.
It now retries every 2 ms for the first 16 ms, every 5 ms until 100 ms, then
returns to 20 ms. Fast retries are confined to that initial window. Context
cancellation, the shared missing-name grace and connection deadlines remain
unchanged. Access denied, broken streams and invalid endpoints are still terminal.
Only opening before authentication is retried; lease and input messages are never
replayed. This bounds one avoidable readiness-detection gap; it cannot remove
the service's real worker preparation or image validation time.

Receiver `pipeOpen` evidence reports open attempt/busy/missing counts and
separate open-syscall and timer-wait microseconds. It is optional, bounded and
validated against the existing PipeOpen parent span. It carries no pipe path,
process id, input or error text, and is unrelated to authority/readiness decisions.

The opt-in g343 recorder also snapshots after a completed source exchange of
at least 80 ms, allowing remaining short stalls to be examined. The request is
issued after handoff cleanup, outside active input; the local-only file cap and
single-snapshot lifetime remain unchanged. Ordinary launches still do not trace.

Validation covers native late-endpoint opening, native owner metadata, cancellation
of successful opens, missing-grace consumption, terminal errors and numeric export
invariants. Field data must distinguish retry waits from native open time before
attributing a remaining pipe span to polling.

## First local runtime-trace inspection

The preceding g343 run saved a 2,447,274-byte trace at
`2026-10-19T08:36:23.5651324Z`. Its SHA-256 is
`4e40afb4180d2c37ecd40f2f901410bb40fdb5ba26618fcbdcb1b3fd8b302659`.
Offline `go tool trace -d=parsed` analysis covers 6.594572049 seconds and pairs
10 GC stop-the-world begin/end ranges. Their maximum is 234,330 ns and total
is 1,316,738 ns. This sample does not reproduce a long GC stop.

The snapshot follows the `peer-session` termination at `08:36:23.4622091Z`
while B was applying g344. Native file/security calls in this window include
update archive reads and transfer work. It is not an ordinary idle-first-entry
sample, and cannot establish which operation caused the earlier field delay.
The raw trace remains local; no source/input/clipboard payload is copied here.

Paired g344 request `25ab718684436950c978539da307d3bd` confirms connected
protected input and no new fault but contains no new handoff. Therefore it
does not prove that the retry change reduced field entry latency. Do not
substitute this idle observation for the pending ordinary and lock/PIN trials.
