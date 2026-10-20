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

## Cross-resident restart admission

The ordinary pair's ExitPending fences replacement within its living resident.
StartPortableProcesses currently starts I/G with inherited R/I process handles,
but supplies no kernel input-ownership reference that a new resident must honor
after R is killed. The production agentinstance lease is held by R, and I/G do
not retain it. Immediate resident restart while G still retires I/recovers the
journal is not covered by the existing parent-death tests or paired health.

This is a static admission gap; no live-device restart/held-input trial was run.
The next priority is a hidden-process trial followed by an ownership fence that
survives R across BOTH I and G until complete retirement. A duplicated mutex
handle must be interpreted by object lifetime, not abandoned mutex ownership;
do not authorize replacement from PID lookup, parent EOF or child exit alone.
Then implement the installed route's independent recovery boundary in separate
small commits. Keep both work items open until their full acceptance is proved.

No native stall was induced on A/B, no hardware result is invented, and no
GitHub comment or remote push is part of this investigation.
