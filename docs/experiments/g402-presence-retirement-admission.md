# g402: deny presence checks during proof retirement (#50/#59)

The production input client's full read-only authority query is context-bounded,
but its following pinned-presence check used a blocking authority read lock.
A concurrent Close waiting for a stalled native reader can therefore also hold
that presence caller behind the writer. The input client invokes this check
while holding its serial wire gate, after the full query and before submission.
Source keepalive and release-child service-presence checks shared the same path.

The presence check now uses TryRLock, matching the physical source authority
admission rule. A closing or busy proof returns ErrAuthority without touching
handles. Available proofs retain their fresh native pipe/process check; the
reader still pins handles until it returns. No cached authority, successful
keepalive, retry, early handle close or input/release capability is added.
The native presence query itself and Close's obligation to join readers remain.

Inputworker, residentagent, peer and application suites and relevant vet pass.
The regression failed before the fix because a heartbeat presence check waited
for the held retirement gate. It covers ordinary/source presence rejection,
fresh checks after gate release and rejection after actual Close. Existing
overlapping native reader and Close-joins-readers tests remain applicable.
This is a deterministic ownership test, not a reproduced Windows syscall hang.

The final #50 call-graph audit also identifies direct protected-input inspection
in the portable resident's press/admission path and remaining native presence
queries. These are not solved by moving SendInput into children. Keep #50 open
until its bounded-dispatch and cleanup claims cover those production paths.
#59 still has physical/queue semantic gates and native desktop/cursor callback
work. No API/DB, dependency, runner, firewall, signing, remote ref or issue-comment
changes. Generation 402 identifies the authorized A/B application bundle.
