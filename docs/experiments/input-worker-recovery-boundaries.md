# Remaining recovery ownership boundaries (#50)

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
