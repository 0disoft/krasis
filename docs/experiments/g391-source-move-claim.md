# g391: separate source sender from producer queue locking (#59)

The source sender previously took queueMu to copy a pending move and clear the
producer's coalescing pointer. Hook/raw producers used the same gate. A paused
consumer could consequently hold a producer behind that copy operation.

The sender now claims the complete X/Y move using one atomic exchange. Move
coalescing uses one CAS against the unclaimed payload. If consumption wins the
race, the distinct new action enters a new frame; it never changes an already
sent frame. The marker cannot collide with normalized +/-32767 deltas. Metadata
and discrete actions remain immutable, and the sender never copies atomics.

queueMu now serializes producers only, preserving multi-producer order, atomic
capacity reservation, split frames, wheel/key/button boundaries and generation
checks. The last pending pointer may reference one claimed item until the next
offer; producers detect its claim before reuse. No CAS retry loop, diagnostic
payload, new goroutine or allocation is added to move coalescing.

## Validation

Passed wininput, agent, inputworker and residentagent suites; repository CI's
wininput vet (-unsafeptr=false) and ordinary vet for the other packages. Focused
tests cover simultaneous claim/merge without torn or lost axes, permanently
claimed frames, correct new-frame fallback, signed limits, zero-total exclusion,
allocation-free coalescing and sender progress while the producer mutex is held.
Existing 125/1000/4000/8000-event coalescing, discrete ordering, generation fencing
and queue overflow tests also pass. Race detection remains unavailable with CGO
disabled. No physical-input, measured latency or polling-rate acceptance is
inferred from those controlled event counts.

## Remaining scope

This removes consumer/producer contention, not all producer/physical semantic
locks. #59 still includes their callback ownership and synchronous desktop and
cursor work. Do not replace fresh native checks with cached authorization or
drop first-input shield semantics. #50 and #70 and the seven hardware/UI/install/
release issues also remain open. Candidate packaging/application is recorded
after the committed build. No push, issue comment, new branch/worktree, external
API, DB, runner, dependency, firewall or signing change is part of this unit.
