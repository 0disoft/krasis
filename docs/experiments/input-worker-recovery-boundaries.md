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

## Ownership primitive checkpoint

`ownership_windows.go` now provides a private lifetime fence using a named
Windows mutex object, with access limited to the current user and SYSTEM.
Acquisition rejects an existing object; it never waits for mutex thread
ownership or treats abandonment as recovery completion. Inherited references
are adopted with inheritance disabled and each wrapper closes at most once.
This primitive is not wired into the product launch or restart path yet.

`TestPortableOwnershipSurvivesResidentAndEitherChild` kills a hidden parent
after two hidden children have adopted inherited references. It verifies that
acquisition remains denied after the exact parent exit and after either first
child drops its reference, in both retirement orders. Acquisition succeeds
only after the remaining reference is closed. These are simulated cleanup
lifetimes, not native input injection or journal-recovery evidence.
`TestPortableOwnershipCloseAndFailedAdmissionDoNotLeak` verifies repeated
failed admission, repeated Close, object disappearance and invalid bootstrap
rejection. Focused tests pass.

The interrupted launcher integration was removed because it supplied an extra
bootstrap argument that the product children did not yet accept. Next: wire
one shared name through resident admission, StartPortableProcesses, BOTH child
bootstrap modes and the full PortableProcesses.Close retirement receipt.
Keep the parent reference alive until native/IPC cleanup completes. Add an
integrated journal-recovery restart trial, including startup failure, before
packaging or deploying. The installed route still requires its independent
authenticated recovery owner. Issue #50 remains open; A/B remain on g391.
