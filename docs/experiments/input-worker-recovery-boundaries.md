# Remaining recovery ownership boundaries (#50)

Current follow-ups: g399-ordinary-recovery-isolation.md and
g400-login-recovery-isolation.md record independent release-only native children
for both installed product journals and paired application. The g394 document
records the earlier login input isolation and g393 job-accounting correction.
Historical remaining-work statements below describe their recorded generations;
use g400's final section for current remaining acceptance.
The single-worker stall below is the pre-g393 diagnosis and remains relevant to
direct/foundation serving and the remaining parent authority/desktop audit.

## Controlled worker stall

TestHeldWorkerRecoveryWaitsForUncooperativeInput acknowledges a simulated down,
then blocks a simulated Move despite cancellation. Advancing virtual time beyond
the recovery timeout does not run key-up or report worker retirement. After the
operation returns, the existing independent recovery context releases the held
input. The test and inputworker vet pass; no native input is injected.

This is supporting evidence of the serving-loop dependency, not reproduction
of a real Windows SendInput hang. bootstrap_windows.go selects that serving loop
with NewInjectionPort for installed input. worker.go retains its pressed state
and deferred ReleaseAll in the same worker execution path. Console retirement
can close its kill-on-close job and prove process exit, but it cannot reconstruct
that worker's journal or create a successful release receipt. workerOwnedPressGuard
does not maintain a second token journal in the resident agent.

The installed route therefore still needs an independent authenticated recovery
owner for a native operation that never returns. Do not wire the ordinary child
launcher into SYSTEM paths or weaken service/image/token/desktop/lease proofs.
An independent guardian must survive input-child retirement, wait for its exact
exit, preserve partial/unknown certainty and remain on the trusted installation
and fixed desktop-policy boundary.

## Cross-resident restart admission (g392)

The ordinary pair now shares the named kernel lifetime fence
`Local\Krasis-Portable-Input-v1`. Its DACL permits only the current user and
SYSTEM. StartPortableProcesses atomically rejects an existing object before
starting input. Both input and guardian inherit one reference through the exact
launch allowlist, disable further inheritance and retain it through serving-loop
and desktop cleanup. The living resident closes its reference only after BOTH
children and every owned native/IPC task have fully retired, including startup
failure. Repeated Close cannot close a possibly reused handle again.

Resident startup also checks the fence while holding its agentinstance lease,
before bootstrap, networking or selecting either ordinary or installed input.
This prevents switching to installed input while an orphan ordinary guardian
still recovers its journal. Object existence, not thread mutex ownership or
abandonment, fences replacement. No PID lookup, parent EOF or input exit alone
grants replacement. This ordinary lifetime reference grants no service, peer,
token or desktop authorization.

## Validation

The hidden-process guardian trial acknowledges a simulated down, kills the
resident, proves exact input-child exit and pauses the real guardian's release
port. Launching a new pair remains denied until the release completes and the
guardian drops its reference. The recorded journal release and successful fresh
acquisition are checked afterwards. Native input is not injected.

Other tests cover either child retirement order, failed input launch, repeated
admission/Close, invalid bootstrap, delayed native/IPC retirement, independently
pending children and ownership-close error preservation. Pair RPC tests use
the inherited-reference bootstrap on both child modes. The crashguard,
inputworker, residentagent and agent-command test/build checks pass.

## Remaining acceptance

The installed route still needs the independent authenticated recovery owner
described above. Issue #50 remains open. No live-device native stall or held-key
restart trial is inferred from hidden fixtures. Packaging and paired application
are recorded separately after validation. No push, issue comments, external API,
DB, runner, dependency, firewall or signing changes are part of this unit.

## g392 paired application

Both PCs applied generation 392, revision
0321587522bdeb72c6bba23d0e4044e0f95d4a54. UI's 85 tests, native builds,
installer provenance and eight-file archive checks passed. Offline verify
accepted 28,073,353 bytes. The packaged agent passed ordinary Default-desktop
heartbeat, cursor read/same-point placement/entry and full paired shutdown.
No keyboard/button injection was part of that smoke.

B receipt a1a4b12d2b18705f3993c87282ac0df7 reports UPDATED. A installer exited
0 and the installed agent SHA-256 matched. Doctor reports healthy/CONNECTED
with matched CLI, UI, resident and installed builds, protected input CONFIGURED
and completed update-stage cleanup. No Disconnect command was sent during A
replacement.

Collection b7610ca19e85ae935e82fe0fe07108c7 at 2026-10-20T22:36:36Z/38Z has
35 A / 58 B current-build events, both READY, one protected capture readiness
each and no fault_ or session_failed. B records replacement-time acquire
timeouts and one RECONNECTING followed by CONNECTED. This short runtime window
does not establish physical-input, held-key fault or DHCP-change acceptance.
Both installed PCs select service input; the ordinary-route proof is the real
agent smoke and hidden-process tests above.

Next #50 unit: the installed bootstrap in inputworker/bootstrap_windows.go
still selects the single worker serving loop and NewInjectionPort. Its
pressed journal/ReleaseAll must survive an uncooperative native operation in
an independently authenticated recovery owner. Keep the protected image,
service/pipe/session/token/desktop and lease checks; the ordinary child mode
is not an installed worker authorization path. All ten issues remain open.
