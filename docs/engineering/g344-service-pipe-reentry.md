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
