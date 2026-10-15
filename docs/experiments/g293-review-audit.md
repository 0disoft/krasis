# External g293 source review: completion audit

Review input: pasted text attachment 0bec0575-1b15-4567-b159-6e07a67f0766.
Audit date: 2026-10-15. Test success below establishes the specified local
properties; device integration remains a separate, incomplete requirement.

| Review requirement | Current implementation and verification |
| --- | --- |
| Lease expiry cannot certify unjoined sender/Interrupt cleanup | SourceSession adds ErrSourceCleanupIncomplete and records independent joins. `TestLeaseExpiryCannotCertifyUnjoinedSourceSender` and `TestSourceCleanupTimeoutMarksUnjoinedInterruptAndFailure` cover blocked owners. |
| Cancellation/revoked generation cannot erase independent send/cleanup failure | Recursive all-leaf classification and the explicit unsent discard marker. `TestSourceSenderCancellationCannotEraseJoinedCleanupFailure` and `TestRevokedGenerationCannotEraseUnknownOrJoinedSendFailures`. |
| Negative admission ACK means only clean GUI inspection rejection | Producer's sourceRetryableInspection gate and consumer matching. `TestSourceNegativeQueryRequiresCleanGUIInspectionFailure` and `TestSourceAdmissionRejectionAllowsAFreshQueryOnSameCaptureOwner`. |
| Reader and executed command results survive cancellation/delivery failure | Independent reader join and first command error. `TestSourceReaderPreservesValidationAndExternalReadFailures`, `TestSourceCommandPumpReturnsTerminalErrorAndStopsOnlyAfterStop`, `TestSourceExecutedCommandAndDeliveryFailuresRemainIndependent`. |
| Buffered negative/wrong reply survives simultaneous EOF | Common waitSourceReply/sourceReplyResult path. `TestSourceClientBufferedNegativeReplyAndEOFHaveOneOutcome`. |
| Missing report/native cleanup failure keeps the service fenced | `TestSupervisorMissingReceiptIsRecoveryUnknown`, `TestCaptureFinalKeepsEveryNativeAndSenderFailureFenced`, `TestRetirementProofCannotHideUnjoinedNativeOrSenderOwners`. |
| Actual Windows capture thread exits before desktop close; m0 excluded | `TestCaptureRetirementWaitsForRealWindowsThreadWithoutInput` and `TestNativeRetiringWaitClassificationPreservesWin32AndNeverInfersExit`, executed separately with GOMAXPROCS=1 and 4. No desktop, window, hook or input mutation in these tests. |
| Bounded source exit/thread/read outcome diagnostics | source_exit.go, report_exit.go, source_reader.go and inputdesktop retirement_failure.go; typed states and append-only numeric report operations. Covered by package tests. |

## Additional cause loss found during audit

Seven source wheel-window setup/foreground/restore wrappers retained their
category sentinel with `%w` but formatted the original error with `%v`. This
discarded errors.Is/errors.As access to the cause and Win32 errno before the
authenticated worker report was written. These wrappers now preserve both causes
with `%w`. Classification, authority, input/capture behavior, retirement status,
wire layout and recovery fences are unchanged.

Before the change, `TestSourceWheelCaptureAppearanceFailureStopsSetup` and
`TestSourceWheelCaptureClassifiesActivationSetupFailure` failed with their actual
mocked native/setup calls. Afterward both pass, together with foreground and
restore tests preserving original failure plus fallback failure. The wininput,
inputworker, logindiag and productlog package tests pass; vet passes with the
existing Win32 unsafe-pointer analyzer warnings excluded. This is a source-only
follow-up to the deployed g304 bundle, not a claim that g304 contains this patch.

## Device gate still incomplete

The user completed the g303 integrated scenario but reported frequent stutter.
A's preserved g303 service failure at 10:29:39 UTC is capture/wheel-capture with
no known native errno. That historical root cause cannot be inferred from the
wrapper defect. Collection `64b87e182e082da48ab6196fb3920b39` confirmed A then
requires service recovery while B's g304 service is READY. Both UI/agents are
g304, but A is CONNECTED/PORTABLE and its installed agent remains g303. This does
not meet the review's starting conditions for a protected-input device trial.

A reboot and automatic installation recovery remain necessary before checking
installed image matches and a fresh continuous A/B lock, PIN login, boundary and
passkey sequence. No service/manual restart, reboot, privilege-policy relaxation,
input replay or raw-input collection was used to bypass this gate.

## Post-reboot update

After the user's reboot, A automatically installed g304 at 14:48:33 UTC and its
new service started at 14:48:34 UTC. Doctor confirms exact g304 UI/agent/installed
agent matching, RUNNING and PROTECTED. Collection
`35e5b69e3c691d171cc406cded3becf9` confirms both services READY, resolving the
earlier reboot gate. A nevertheless timed out awaiting an idle source heartbeat
response at 14:49:49 UTC and reconnected. The continuous-device verification gate
is still open; g305's narrowly guarded idle-source retirement is documented in
`g305-idle-source-heartbeat-retirement.md` and requires device validation.
