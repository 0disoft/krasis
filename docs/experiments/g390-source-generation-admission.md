# g390: remove source state waits from action admission (#59)

SourceSession.Offer previously took stateMu.RLock on the physical keyboard,
button and wheel paths. The same read lock was used by the sender's final
generation predicate under its semantic wire gate. A concurrent state writer
could therefore delay those paths even though Offer claimed never to block.

The generation is now one atomic value, not a second cached copy. Activation,
deactivation and local return still serialize their writes and publish the same
active/terminal invariants. Offer and the sender predicate read these atomics
without waiting on state transactions. After queue admission, Offer rechecks the
captured generation so a paused producer cannot enter a later lease. No new
contention-triggered abort or recovery is introduced.

Physical held-input ownership, activation's fresh native inspection, ordered
queue/coalescing, emergency release, current-desktop checks, first-input shield
hiding and native-owner retirement retain their existing contracts. Queue and
physical semantic locks still exist; this is not full #59 completion or a claim
that every callback operation is nonblocking.

## Validation

The targeted regression holds the state write lock while physical key down/up,
button down/up and wheel actions are admitted in order, with no forced recovery.
It also checks the sender's current-generation predicate under that lock. A
paused producer is rejected after deactivation/re-activation, while a fresh
action remains admissible. Local return invalidates the old generation before
cleanup and cannot reactivate it after completion.

Wininput, agent, inputworker and residentagent suites passed. Wininput vet uses
the repository CI's -unsafeptr=false setting because native callback trampolines
in unchanged files trigger that analyzer; default vet for the other three
packages passed. Race detection is unavailable with CGO disabled. No physical
input or long soak is inferred from these controlled tests. Candidate packaging
and paired application are recorded separately after the source commit.

## Next boundaries

Continue #59's physical/queue gates and synchronous desktop/cursor work without
using stale desktop authority or admitting the first click through a shield.
Keep raw/hook progress retirement separate from idle input. #50's full native
boundary audit and #70's physical endpoint change remain open, as do the seven
hardware/UI/installer/release acceptance issues. No issue comments, push, extra
branch/worktree, external API, DB, CI runner, dependency, firewall or signing
change is included in this unit.
