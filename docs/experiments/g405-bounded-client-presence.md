# g405: bound receiver client presence queries (#50)

Client.performResponse performed a synchronous pinned-process presence check
after taking its serial wire gate. The existing full-authorization timeout did
not bound this second native query. A regression with a 50ms caller and a native
presence query released at one second failed before the fix: the caller waited
the full second despite submitting no input.

Presence now uses an independent read-only query lane owned by Channel. It uses
the command's existing deadline, shares the four-query process capacity with full
authorization, and retains pipe/proof handles and its capacity slot until native
return. Close fences both lanes immediately; only the last returning owner can
close shared resources. Native results arriving after cancellation cannot admit
input or revive the channel. An expired presence check preserves not-sent
certainty, the unconsumed sequence and the context deadline error. No command is
retried and no previous success is cached.

Presence cannot queue behind a slow full SCM query: its separate lane permits
real worker heartbeat writes while full authorization is pending. Both lanes
continue to require a fresh process proof and share existing authority lifetime
locking. This change is limited to receiver Client calls; other source/native
callback and worker-side presence paths still require their own audit.

Verification: full inputworker tests and vet pass. The new virtual-time tests
cover input and heartbeat presence expiry at 50ms, zero wire writes/sequence
consumption, retained native handles, and exactly-once retirement with concurrent
full/presence queries in both completion orders. The existing
TestSlowInputAuthorizationDoesNotStarveWorkerHeartbeat passes with two actual
worker heartbeat writes while full native authorization is stalled. Existing
full-query capacity/late-result and constructor tests also pass. These controlled
tests do not establish native held-input cleanup or physical latency acceptance.

Generation 405 also includes the separate installation directory diagnostic
unit documented in install-directory-move-diagnostics.md. #50 stays open pending
the remaining production call graph and held-input acceptance; #16 is not closed
by improved diagnostic codes. No API/DB, dependency, runner, firewall or signing
policy changes; no push, issue comment or remote Git ref mutation.

## Paired application

UI 85 tests, native builds, embedded installer provenance and eight-file bundle
checks pass. Offline verification accepted g405 revision
0756c06c46c3c81f213b5fe5f35af979a21bb06c, 28,297,014 bytes. A's existing UI
performed its verified replacement handoff. Installation completed at 00:50:20Z
with result 0 and no recovery needed; its installed agent SHA-256 matches the
bundle. B transfer 8b2bf2e9e6be85c5265855c389b89092 reports UPDATED; B's installer
completed at 00:50:55Z with result 0 and no recovery needed.

Collection faf0730e02d2322d4e6eed244dcd7113 at 00:51:09Z contains 23 A / 24 B
current-build events, READY service/input health, connect_started and CONNECTED
on both devices and no current-build fault/startup-failed event. Fresh A status
and doctor confirm CONNECTED, matching CLI/UI/agent/installed revisions, verified
launch record and completed stage cleanup. B's connection resumed during the
update without the previously requested manual Connect action; the g404 pending
connection observation is superseded by this g405 evidence.

An extra installed-input smoke could not run concurrently with the normal
resident: it exited 1 with the singleton message that an agent was already
running in this user session. This is not a passed smoke. The healthy paired
session was preserved instead of stopping it solely to repeat the historical
cursor/same-point placement smoke. No current-build held-key, security-desktop,
power, DHCP, home-pair, accessibility/DPI, soak or signed-release acceptance is
inferred. CGO was disabled, so race detection was not run; hosted CI was not run
because commits remain local.

The first recovery-registration command used the distribution directory while
the active UI was running from its verified staged directory. Doctor detected
that path mismatch. Running app repair from the active verified stage corrected
the launch record; the subsequent doctor reports all checks healthy.

Completed this continuation: installation-stage diagnostics (7987833) and bounded
receiver presence (0756c06). No additional implementation unit was started.
Next: complete #50's remaining source/worker authority and native cleanup audit,
then #59's native callback dependencies. Open issues remain
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70; physical device-pair identity and real signing
availability still need the earlier requested external evidence. Only main and
its existing origin/main tracking reference are present; no branch/worktree or
remote ref was created, merged or removed in this continuation.
