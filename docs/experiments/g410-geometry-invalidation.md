# g410: reject geometry results after session retirement (#50/#59)

The production geometry guard checked its permanent invalidation bit and session
context only before CurrentDisplayGeometry. Two controlled tests failed: a read
that canceled its session still returned success, and a read returning the old
geometry after a concurrent caller invalidated it also returned success. The
guard participates in source boundary and destination entry validation.

After a matching native result, recheck permanent invalidation and cancellation.
Changed/unsupported geometry still invalidates exactly once and invokes the same
session-stop callback. No geometry result is cached, no input is replayed, and
the native read stays on the original caller. This fixes stale admission rather
than claiming to cancel a Windows native query.

The callback audit confirms capture scopes use a fixed desktop binding whose
Scope.Do checks live UOI_IO, without SCM/WTS on every mouse move. Immediate native
desktop checks and visible return-cursor shield hiding remain necessary; moving
them to detached work would not preserve desktop/first-click correctness. No
speculative change was made to these native boundaries.

The two geometry regressions fail before the fix. Focused geometry tests, relevant
package verification, build and paired application are recorded below after
execution. User confirmation of g377 ordinary input is not current-build held-key,
power, secure desktop, home-pair, DHCP, accessibility or signed-beta evidence.
No API/DB, dependency, runner, repository hygiene, firewall or signing change.
No push, issue comment, remote ref edit, additional branch or worktree.
