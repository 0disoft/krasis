# g393: keep installed recovery outside the native input child (#50)

Correction: the full-suite run during g394 exposed that a job accounting count
of zero can precede a native child's signaled process handle. g394 now requires
the authenticated final recovery receipt plus the exact parent exit; accounting
alone cannot clear missing child-retirement proof. See
g394-login-native-isolation.md. The g393 rollout below remains historical evidence.

The authenticated console worker now retains its pressed-state journal and
recovery desktop while a distinct job-bound native child performs live input.
A timed-out native operation fences/kills that exact child. Before any recovery
key-up, the worker joins the child's full process/native/IPC retirement. An
unconfirmed retirement preserves Remaining and ErrRecovery and performs no
release, including an empty-journal session. Applied/partial/unknown input
certainty continues through the existing RPC decoder and pressed-state logic.

The native child uses one fixed protected executable/mode, with no selectors
in arguments or environment. Four distinct inherited references carry the
agent pipe, pinned agent image, service pipe and exact console parent. The
child independently verifies installation, SYSTEM/UIAccess console token,
SCM/service pipe/image, agent pipe/image/logon and the protected live parent
before accepting a lease. Each input still checks fresh authority, fixed
interactive desktop, parent presence and immutable expiry. Source capture,
cursor presentation and login-worker paths are separate and unchanged.

The service keeps a two-process job for ordinary console workers. Retirement
terminates that exact job and retains its handle until accounting proves zero
active processes, then observes/closes the owned parent reference. Parent exit
alone cannot release a native child into the next session. Login jobs retain
their one-process limit. Failed native handle operations poison ownership
instead of retrying potentially reused values.

## Validation

Passed crashguard, inputworker, residentagent, input command and agent command
test/build checks and ordinary vet. A hidden subprocess acknowledges a simulated
down and then blocks Move without cancellation. The real local transport times
out and retires it; the independent serving worker releases its journal only
after exact child exit. The test checks a successful release and an outcome-
unknown move. Empty/held unconfirmed retirement tests reject recovery. Native
job tests terminate/join two real hidden processes and check the second process
cannot survive replacement admission. Bootstrap tests reject missing,
non-inherited, duplicate and already-adopted references. No native key/button
injection is used in these fixtures.

`installed-input-smoke-internal` is a fixed local maintenance mode in the real
agent. It holds the resident instance lease exclusively and can only exercise
service/native-child cursor read and same-point placement. It has no peer,
path, desktop, key or button selector. An uninstalled executable cannot obtain
the service's fixed agent-image proof. This mode supports attended installation
validation; paired rollout results are recorded after application.

## Remaining scope

This isolates native child operations, not every synchronous authority or
desktop query in the journal-owning worker. Those parent-side boundaries and
the separate login worker still need review before closing #50. Native-stall
fixtures do not reproduce a Windows SendInput hang or replace physical held-
input/fault-matrix evidence. No push, issue comment, external API, DB, runner,
dependency, firewall or signing change is part of this unit.

## Paired application

Both PCs applied generation 393, revision
5cace495a8a1bdaf83357fbc6d436cd8119f57f6. UI's 85 tests, native builds,
installer provenance and the eight-file archive check passed. Offline bundle
verification accepted 28,278,515 bytes. A installer exited 0 and its installed
agent SHA-256 matched. Before restarting the UI, the actual protected agent's
fixed maintenance mode passed service/native-child cursor read, same-point
placement and shutdown. Afterwards only the session-0 service process remained;
both console processes had retired. This uses the real privileged launch,
inherited-handle bootstrap and independent authenticators, not a portable proxy.

B receipt bc27e9bb8e8ae701590a51909206fd89 reports UPDATED. Doctor reports
healthy/CONNECTED with matched CLI, UI, resident and installed builds and
protected input CONFIGURED. Peer status reports ONLINE/g393 at the same
revision. No Disconnect command was sent during replacement.

The B replacement closed A's prior peer session at 22:52:09Z, with
fault_peer_session_failed in its trigger detail and final LOCAL state. A was
explicitly reconnected at 22:53:14Z. This interruption is retained, not reported
as a fault-free rollout. Collection 09197c8028735820fc90e6d54de4465e at
2026-10-20T22:53:23Z/25Z has 65 A / 55 B current-build events and both READY.
No new fault or triggered session-end detail appears after that reconnect.
This is a short observation window, not soak, physical key/button/scroll or
real Windows native-stall acceptance. A g393 three-round-trip physical test
request is pending; the user's earlier g377 pass is not reused for new routing.

Next priorities are the journal owner's synchronous authority/desktop work,
the separate login-worker recovery boundary, then the remaining #59 callback
and #70 endpoint-change evidence. All ten issues remain open until their own
acceptance is supported. No new implementation unit started after the
ten-minute cutoff. All source and rollout commits remain local.
