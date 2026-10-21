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
