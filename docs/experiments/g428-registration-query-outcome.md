# g428: distinguish unobserved registration from a confirmed mismatch

An actual g427 screenshot shows an app recovery registration warning while a
Windows Security firewall permission dialog blocks UI interaction. Independent
CLI recovery-status reports task ENABLED; the task target matches g427. This
does not identify the exact earlier GUI read error or prove every registration
artifact healthy. The screenshot is not accepted as an unobstructed UI review.
No security-dialog input or firewall setting change is performed.

AppSettings currently maps both an OS registration query error and a confirmed
registration mismatch to startup_failed/recovery_failed. A read timeout or
access failure therefore claims a failed registration without observing one.
A new injected-query regression fails both query-error cases on the original
source (full focused result 0.169s); mismatch, rollback-unknown and verified
cases preserve their existing meanings.

g428 projects startup_query_failed/recovery_query_failed for read errors. The
existing generic settings-verification guidance handles these outcomes without
showing raw errors. Confirmed mismatches keep their registration-failed codes.
Known saved preferences remain known and the frontend bridge preserves their
automatic-update guidance; an OS query error cannot rewrite user preferences.
Rollback uncertainty still takes precedence as rollback_failed. No read path
writes registry/task state, clears failed-write latches or grants new authority.

The full Windows/desktop workspacehost suite passes in 0.449s and all 86
frontend tests pass. The new frontend case verifies allowlisted projection,
raw-error exclusion and retained automatic-update guidance. This fixes a proven
classification defect; it does not add retry loops or claim the earlier GUI
warning's exact cause. Opening Settings already provides a new read attempt.

The local UI projection adds two bounded code values; old/mismatched frontends
retain their existing unknown-code fallback. No agent/network wire schema, DB,
dependency, runner, input path, signing, firewall or repository-hygiene change.
Product-control documentation records the distinction. Native packaging and
application results follow separately. No push or issue comment.

## Packaging and paired application

Local source 90706b73f85217fb459aaff6fa55f78ff77c3c74 is packaged as g428.
The workspace builder passes 86 UI tests, native frontend/executable/embedded
installer builds, real update archive smoke (6.63s) and eight-file verification.
Offline verification accepts the 28,212,141-byte bundle. The existing input
implementation is unchanged from g427, so its full suites are not repeated.

B job 097b87692e938d61d6b354a611bc227a reaches UPDATED. Its installation
receipt is result 0 at 12:44:19.4595716Z. A waits for actual exit of both
path-verified UI/agent process handles and disposes them before repair; its
installer returns exit 0/result 0 at 12:45:14.1064048Z. A app repair/relaunch
registers and opens g428. Doctor reports matching UI/resident/installed g428
revisions, healthy CONNECTED protected input, service RUNNING and complete
update-stage cleanup. No Windows Security prompt is operated during application.

Collection 691808a1e146a68dd9be20a0f9526cd3 at 12:45:40Z/12:45:42Z confirms
both result-0 receipts, both services READY and both inputHealth states READY.
This is successful application evidence, not a physical latency/soak claim.
The earlier g427 protected connection also remained healthy in the 12:42Z
doctor observation, but unattended health cannot close the physical matrix.

## Remaining external prerequisites

The live issue bodies were re-read after application. #1 remains excluded.
The other eight issues still require evidence that this round does not provide:

| Issue | Missing required evidence |
| --- | --- |
| #4 | Human-operated study-pair admission/input reports and exact-revision CI kit. |
| #5 | Identified home pair, redacted device inventory and attended bidirectional probes. |
| #6 | Manual power/session operations, physical held-input/termination/recovery matrix. |
| #7 | Physical devices/rates/DPI/privilege combinations, measured distributions and soak. |
| #15 | Remaining actual GUI/state/accessibility/high-DPI review, publication and hosted CI. |
| #16 | Publisher signing plus complete attended install/remove/firewall/data/rollback acceptance. |
| #20 | All linked issues, four-device acceptance, default-branch CI and signed beta candidate. |
| #70 | Actual two-PC endpoint address change and authenticated recovery without unnecessary unchanged-address DHCP disconnect. |

CurrentUser/My and LocalMachine/My contain zero currently valid private-key
code-signing certificates. The configured file/thumbprint/timestamp variables
are absent; external signing-service availability is unknown. Only counts and
presence booleans were inspected. A user question requests a signing environment
name/configuration location, never a key/password.

GUI interaction stops at the observed Windows Security permission overlay.
The user handoff to handle that dialog remains pending; silence is not consent.
The classification correction is tested and applied, but a post-change GUI
review is not claimed. Direct terminal application did not change permissions.

The same hardware/signing/publication prerequisites remained unresolved through
consecutive g425/g426/g427/g428 rounds. Local improvements do not satisfy them.
No further independent source defect is established in this round. The next
work depends on user/external evidence, including the two pending handoffs;
do not generate more builds, fabricate reports, weaken criteria or close these
issues merely to empty the tracker. Existing no-push instruction still applies.

## 2026-10-22 physical-use follow-up

The user reports that B-side movement is much improved, with almost no stutter,
but a slight catch remains when crossing into B. This is current g428 ordinary
use feedback, not completion of the device/rate/privilege or lifecycle matrix.

Fresh paired collection `55c0dfc102811f65c0c8a7e51835594f` is saved locally at
09:34:21Z/09:34:26Z. Filtering event timestamps after the preceding collection
at 09:31:54Z retains 12 source and 12 receiver profiles, all ending `ok`, with
1,663 source dispatches and 1,453 receiver dispatches. Counts differ because
receiver pointer coalescing is enabled; they are not a lost-input count.
Both reports retain service READY and input health READY. No triggering
session-end event appears in that filtered interval. A's final doctor reports
healthy CONNECTED and matched g428 UI/resident/installed builds.

Receiver ReadyWait spans range from 115,000 to 137,007us, whereas the maximum
entry dispatch is 9,321us and the maximum ordinary dispatch is 34,889us.
Source ordinary dispatch peaks at 4,062us. These are process-local measured
spans, not physical input-to-display latency. The entry wait is a separate
remaining target from continuous movement; the current evidence does not
establish its exact removable cause or justify skipping readiness checks.

The earlier collection `2433774db9a59a92ce25f57d257635d6`, at
09:31:49Z/09:31:54Z, retains a receiver dispatch of 1,018,280us, including
96,559us of client authorization and 921,720us of response wait; its matching
worker apply measurement is 917ms. Another dispatch is 861,098us with an
858,663us response wait and 350ms worker apply measurement. These nested
spans must not be added or subtracted to claim a native or network delay.
The later sample and user report do not erase those earlier outliers.

Next performance priority is a bounded breakdown of remaining worker readiness
work during crossing, preserving fresh authorization and held-input cleanup.
No new source defect is reproduced in this inspection, so no code change,
new bundle, device replacement or issue closure follows. Signing, hosted CI,
full device acceptance and pending security-dialog handling remain incomplete.
Only this observation record changes; no API/DB, runner, dependency, security
configuration or branch change. No implementation tests are rerun for this
documentation-only update; validation is `git diff --check`. No push or comment.
