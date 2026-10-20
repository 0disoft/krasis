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

The callback audit also narrows the next native investigation: protected
low-level physical callbacks call sourceLoginDesktopCheck/Scope.Do on their
fixed capture desktop binding. That path reads current desktop activity; it
does not execute a full SCM/WTS proof on every mouse move. Full service/agent
checks remain on source commands, startup and transport admission. Do not remove
or cache the live desktop check to fix an assumed duplicate authority query.

## Paired application

Both PCs applied generation 390, revision
6233ccca81eef0eb482b35219576e170f315db7b. UI's 85 tests, native builds,
installer provenance and eight-file archive checks passed. Offline verify
accepted 28,061,680 bytes. B receipt 0f837488023efd7e285bd601b18d89be reports
UPDATED. A installer exited 0 and installed agent SHA-256 matched. No Disconnect
command was sent during replacement.

Doctor reports healthy/CONNECTED with matching UI, resident, installed build
and recovery record. Collection 04cddcb0a945246deec7cc0109e3aaf2 at
2026-10-20T22:10:02Z/03Z reports READY on both PCs and one
installed_source_capture_protected_ready event each. Its 26 A / 49 B current-build
events contain no fault_ or session_failed. B recorded one RECONNECTING while A
was replaced, then reconnected. This short startup window is not physical input,
latency, polling/DPI or soak acceptance. All ten issues remain open.

No new implementation unit was started after this round's ten-minute cutoff.
Next work remains the physical/queue semantic gates and synchronous cursor work
under #59, followed by the native-boundary acceptance under #50. Source and
rollout evidence are local main commits; no push or issue comments were made.
