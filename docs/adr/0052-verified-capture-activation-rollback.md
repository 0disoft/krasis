# Verified capture activation rollback

Status: accepted

## Context

Generation 19 could become FAULTED after a foreground activation rejection even
when rollback succeeded. The native capture owner treated every activation error
as terminal, although the outbound controller already aborts a rejected grant
and retains the connection with local input ownership.

## Decision

Generation 20 distinguishes a verified clean activation rejection from an
unverified native cleanup. Only the capture owner may create the private rejection
type after unregistering capture, verifying the window is hidden and no longer
foreground, and checking inactive state, no thread-detach failure, and no terminal
deadline/cancellation. This rejection leaves the native owner reusable; the
existing LEASE_ABORT path returns control locally without reconnecting. Abort
write or runtime failures remain errors.

Any failed or unknown cleanup still terminates the capture owner and preserves
the recovery-evidence FAULTED policy. This is not a blanket retry of native errors.
Already owning the foreground is accepted even when the request API reports
failure. Foreground absence, unavailable thread, responsiveness probe failure,
and rollback failure receive separate fixed diagnostic codes. Activation failures
are logged even when the transport remains connected; no free-form error or
foreground application identity enters the support export.

## Verification and limits

Mocked native tests cover reuse after rejection and denial of recovery for each
missing cleanup proof. Related race/checkptr tests cover the control and logging
paths. No real hook or foreground manipulation is required by these tests.
Two-device foreground contention remains a manual test. A responsiveness probe
does not guarantee a later native call cannot stall; existing deadlines and
unknown-owner fault handling remain necessary.
