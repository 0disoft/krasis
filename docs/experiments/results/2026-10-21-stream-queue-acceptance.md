# Ordered stream and source queue acceptance — 2026-10-21

## #49: bounded stream-stall recovery and attribution

Local main already records byte progress, complete-frame receipt, dispatch,
injection and coordinator waits separately. Handoff profiles include measured
TCP RTT, retransmitted-byte and timeout-episode deltas when available. Source
movement is coalesced before serialization; ordered TCP bytes are never bypassed
by a later heartbeat.

The focused peer suite passes delivery pauses before the first byte and inside
an input payload, receiver read suspension, and injection-versus-read progress
attribution. The additional
`TestOrderedStreamCancellationReleasesHeldInputBeforeHeartbeatCanArrive` sends
one key-down, stops delivery before the heartbeat and cancels the session. It
verifies that cancellation interrupts the blocked reader, releases the exact
held token and returns to LOCAL without validating an undelivered heartbeat.

Close the bounded-recovery/diagnostic implementation issue #49. TCP head-of-line
blocking remains a property of the selected ordered transport. These tests are
controlled stream stalls, not packet-loss/retransmission/zero-window experiments.
Hardware network and latency characterization remains part of #7 and the full
fault matrix remains #6. Do not attribute EOF to wireless packet loss.

## #57: bounded movement backlog

The current source queue merges only consecutive movement within one generation.
Key/button/wheel and generation boundaries remain ordered. Synthetic blocked-
sender tests pass for 125, 1000, 4000 and 8000 movement events with pending depth
one and the exact aggregate movement. Overflow and send failures disable source
suppression; retired actions cannot enter a new lease. Queue size was not raised.

In the fresh paired g368 collection described in
`2026-10-21-trust-regression-acceptance.md`, A's 32 runtime summaries account for
4,748 queued source actions, maximum queue depth three and no overflow event.
B records 3,838 completed injections. Source owner exit records show joined
capture/sender/interrupt owners even for the separate IPC failures. These are
ordinary-use observations, not a controlled high-polling hardware benchmark.

Close #57's queue-overflow implementation defect. Actual device polling rates,
latency distributions and soak/support limits remain explicitly unvalidated in
#7. This retires duplicate implementation tracking without claiming that the
physical-device matrix is complete.

## Validation and boundaries

Passed with repository-local caches, `GOPROXY=off` and `-count=1`:

```text
go test ./internal/peer ./internal/wininput -run 'Test(OrderedStream|PausedReceiver|ProgressSeparates|DispatchTimingDistinguishes|SourceSession)' -count=1 -timeout 45s
```

Only tests and acceptance records changed. No new generation/device replacement,
product API, DB, dependency, runner, hosted CI, release or Git push was needed.
The recurring protected-source IPC failures remain unresolved; see the trust
acceptance record for the separately observed failure times.
