# Source activation admission and worker timing

The paired g331 collection `c488c1d0bd9f71d659665d5f0ea4fd75` contains eight
matched source/receiver exchanges during 2026-10-17T09:55Z. All completed with
`end:ok`; receiver entry dispatch took 2.3-4.1ms with prepared children. Source
capture activation took 52-577ms, almost entirely in the local source request's
response wait (51-568ms). These are nested spans, not additive. No new capture
failure was recorded in this roundtrip interval. The preceding reconnect and
older capture failures remain separate incidents.

## One full activation check at execution admission

The protected source reader previously called both full agent/service checks
before enqueuing an activation. The dedicated command desktop then repeated
those full checks before executing it, and the native capture's boundary guards
checked again around activation. Full checks can include synchronous WTS/SCM
queries. This repeated path is established by code, but its contribution to the
observed 52-577ms is not individually measured in g331.

G332 admits only activation and the already presence-only heartbeat using the
immutable authenticated owner checks. Activation still must pass the protected
command desktop's full live authorization immediately before native work;
queued revocation is rejected there. The fixed protected hook owns that desktop
executor. Its native boundary guards, physical pressed-state checks, desktop
activity checks, rollback, exact sequence/epoch and response rules are unchanged.
All other commands retain their read-side checks, including stop/deactivation
and cursor restore. Heartbeat's presence proof still cannot activate capture.

The deterministic test injects an 80ms full query. Activation frame admission
performs zero full queries instead of one, and execution still observes a
revocation introduced after enqueue. This establishes removal of one redundant
query per activation, not an 80ms device speedup or a fix for all GC stalls.

## Timing tied to the exact response

Three previously reserved bytes of the fixed 64-byte private source packet now
carry the existing 0-5 timing buckets on activation replies only. Other packet
kinds/operations reject nonzero timing; invalid buckets and remaining nonzero
padding are rejected. A zero legacy reply remains unobserved. The first bucket
is currently zero: reader authorization is not measured as a dispatcher phase.
The second is command scope, including its live authorization/desktop selection,
excluding the callback. The third is that same command's native callback,
including existing boundary guards, physical-state checks and window activation.
It is not solely an OS window-call duration.

The client consumes timings only after the exact reply/op is matched. A matching
negative reply retains measured phases and its rejection outcome; foreign replies
remain protocol errors and contribute no worker timing. Existing 15-second
maximum buckets publish `installed_source_timing_worker_scope_*` and
`installed_source_timing_worker_apply_*`. These maxima can describe different
commands and must not be summed or matched to a particular handoff. A missing
apply bucket on scope rejection means native work was not observed. Reply-wire
serialization, read scheduling and command-queue wait remain outside these two
worker spans and inside the client's response wait.

This private source extension requires the ordinary matching protected-agent
installation; peer wire/public APIs are unchanged. It adds no key/button kind,
text, coordinates, PIN, window identity or error text to diagnostic records and
no per-input logging. GC tuning, deadlines, lease expiry and input replay rules
are unchanged. No DB, dependencies, ACL, settings, or runner changes. Rollback
uses a matching application/protected-input update, not mixed local executables.

## Verification

Tests cover removed read-side query cost, queued revocation, unchanged checks on
other commands, cleanup without acquiring an inactive desktop, one native apply
with the original failure/deadline, separate scope/native buckets, malformed and
misplaced packet timings, and foreign versus matching negative reply handling.
Existing inputdesktop/protected-owner tests continue to cover wrong-thread,
ordinary-caller, revoked scope and failed native-cleanup rejection. Actual
roundtrip and lock/PIN behavior require a fresh paired-device observation after
both versions are applied.
