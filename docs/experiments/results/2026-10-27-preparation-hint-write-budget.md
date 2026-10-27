# Preparation hint write budget

The g469 paired analysis is in `2026-10-27-g469-grant-delay-separation.json`.
Thirteen successful A-to-B exchanges have source grant-wait median 227.935ms,
receiver dequeue-to-grant-write median 37.355ms, and paired unassigned remainder
median 67.654ms. The medians are independent distributions and are not additive.
The longest source wait is 616.661ms: receiver work accounts for 36.227ms and
580.434ms remains transport plus unobserved application queues/scheduling.

Independent ICMP to the established input socket's actual remote endpoint has
32 successes, median 117ms and maximum 694ms. The gateway probe has 15 successes
and one timeout, with maximum 144ms. The probes were separate from the physical
run; they do not identify which radio, driver, power policy or AP caused delay.
The saved alias address equaled the host address, so self-ping results were
discarded. No network or power setting was changed. These measurements do not
justify a claim that all handoff delay is network delay.

## Corrected runtime behavior

The optional preparation sender used a 500ms admission context but passed its
long-lived session context to the actual frame writer. A blocked write therefore
retained the semantic gate until the transport's independent 2s write deadline,
potentially delaying actual lease traffic behind a stale optional hint.

Admission and writing now share the hint's original `at + 500ms` deadline. An
already-200ms-old hint has at most 300ms left, including admission. A TLS write
that has started still fails the session on timeout because its outcome may be
partial; it is never treated as a safely dropped hint. Pre-admission expiration
remains nonfatal. Sustained congestion can therefore cause earlier safe reconnect
rather than a 2s gate stall. No timeout was relaxed and no cleanup receipt bypassed.

## Verification

- The deterministic regression fails against the original implementation with a
  2s gate hold in both fresh and 200ms-old cases. With the fix it passes with
  500ms and 300ms respectively. This is a bounded-gate reproduction, not evidence
  that this hint caused the longest observed g469 request wait.
- An 80ms receiver read-ahead backlog is now reported as controlQueue, separately
  from dequeue-to-grant work. A control barrier kept by pointer batching retains
  its original read timestamp. Source matching, stale/missing evidence and export
  reprojection/bounds are covered too. Existing exchange indices are unchanged.
- `go test ./internal/agent ./internal/peer ./internal/handoff ./internal/productlog ./internal/transport -count=1 -timeout=180s` passed.
- `go vet ./internal/agent ./internal/peer ./internal/handoff ./internal/productlog ./internal/transport` and `git diff --check` passed.
- Race detection was skipped because the active toolchain has `CGO_ENABLED=0`.
- Physical acceptance of the new code, long soak and package installation were
  not run. Both installed laptops remain g469. No physical speedup is claimed.

Only runtime bounds, additive diagnostic data and experiment documentation are
changed. The diagnostic source of truth is `docs/experiments/handoff-profile.md`.
Public API, peer wire format, DB, dependencies, repository hygiene policy, CI and
runners are unchanged. No generation bump, push or deployment is part of this run.
