# Prepared-worker service readiness: policy I/O contention

## Evidence and scope

The g470 paired export retained one warm B input child, age 1,366,216us,
with a 390,802us agent-side readiness wait. Its service bootstrap was
73,852us, including 37,327us of child desktop readiness. The remaining
316,950us is unassigned; these nested spans do not prove which service
operation or OS scheduling delay consumed it. Its preparation join was an
observed zero. See `2026-10-27-g470-physical-acceptance.json`.

Source inspection found an avoidable blocking path before that bootstrap:
the 250ms login policy poller reads and validates protected grant metadata
while holding the same `loginServiceGate.mu` used by normal input admission.
A ready child cannot bypass that gate. Three fixed paths are opened/checked
per grant read, with further reads when a grant exists. The lock can therefore
make normal admission wait for unrelated filesystem or ACL-query latency.

## Change

The single policy poller now reads its selection before acquiring the gate.
Only `Owner.Reconcile` and input admission share the gate. This removes
policy filesystem I/O from the admission critical section; the polling rate,
fresh console checks, grant validation by worker startup, retirement join and
failure fencing are unchanged. A delayed selection is scheduling data, not a
cached authorization. It must still pass the owner's fresh console observation
and independent startup validation. A read error still disables the selection
and retires any old pre-login owner under the gate.

The change does not eliminate console/WTS calls inside owner transitions,
direct startup checks, pipe waits, or OS scheduling. It cannot establish that
the measured physical 391ms case was caused by the policy file read.

## Verification

Before the fix, injecting 317ms into the policy read made authenticated
admission wait 317.3965ms. A second regression held the read until foreground
admission completed; the original code could not admit during its one-second
observation window. After the fix, admission can finish while that read remains
blocked, and the delayed selection cannot start a pre-login owner after logon.
Existing cleanup-failure and retirement-order tests remain applicable.

This is an injected blocking-path regression, not a measured physical speedup.
No generation bump, installation, push, API, DB, runner or hygiene policy change
is part of this change. Physical acceptance requires a later coordinated install.
