# g411: serialize short installation observations with replacement (#16)

g410 had two A folder-retirement failures (Win32 5 then 32), each restored,
before UI closure/resident join allowed installation. That intervention did not
identify the live handle owner. Audit found a separate reproducible mechanism:
health/launch code checked the exclusive transaction lock, released that check,
then pinned installed images. Doctor skipped the transaction check entirely.
An installer could acquire its lock between the check and image pinning.

A Windows file-handle regression failed with the old check-then-load behavior:
the exclusive writer entered both during and after the observation's load.
Short-lived health/build observations and launch pinning now hold a shared-read
handle on the fixed transaction lock before opening any image. They release it
only after image/metadata pins close. Observations cannot overlap an installer,
create a missing lock, or change the lock/journal. The actual installation loader
still independently checks ACLs, content, digests and pinned identities.

The installer waits at admission for these readers, under its caller deadline
and a two-second maximum. A probe distinguishes live exclusive transactions,
which remain immediately rejected, from shared readers. Exclusive CreateFile is
still the admission decision. No transaction/staging/service stop/replacement
has begun during this wait, and no mutation is retried. Service/native worker
lifetime owners retain LoadInstallation rather than the observation guard:
the installer must acquire its lock before stopping and joining those owners.

Controlled native tests cover writer exclusion throughout an observation, image
pin closure, refusal to load during a writer or when the lock is missing, cleanup
after load failure, admission after a reader returns, two-second reader expiry,
earlier caller expiry and immediate rejection of a concurrent installer.
Inputworker/productcli/productpanel suites and vet pass; final focused additions
and build/application results are recorded below after execution.

This eliminates the demonstrated race, not proof that a particular g410 failure
was caused by doctor or a UI reader. No image sharing, ACL, service authentication,
rollback or native cleanup policy is weakened. #16 also retains signing/full
device lifecycle criteria; #50/#59 retain their native/physical evidence gates.
No DB, runner, dependency, firewall or signing change. One local read-only
observation API was added within the application; no peer/control wire schema
changed. No push, issue comment, remote ref, branch or worktree was created.

## Pre-application completion: g412

Final doctor call-graph inspection found process-location reporting reopening
the installed image's build after closing its observation guard. It now reuses
the verified observation from that same doctor invocation, while failed/unknown
observations stay unknown. Portable bundle paths retain their ordinary read.
This is diagnostic snapshot reuse, not cached input authority. The regression
prohibits reopening the installed path, including case variants and failed
observations. g411 was built/verified but never applied or sent; g412 supersedes
it before paired installation as completion of this same observation unit.

## Verified g412 application

Inputworker/productcli/productpanel suites and vet passed. The final focused
native observer/writer tests and productcli suite/vet passed after completing
the caller-budget/error-preservation and doctor paths. UI 85 tests, native build,
installer provenance, archive test and eight-file package verification pass.
Offline verification accepts g412 revision
92cb4c880d5aa2fc3bd0fb793529081adf1565e7, 28,208,602 bytes. Source units are
d48ea1d and 92cb4c8. No race detection with CGO=0, and no hosted CI because push
remains forbidden. No standalone input smoke was run beside the healthy resident.

A installation at 02:08:44Z and B at 02:09:42Z both report complete/result 0,
recovery not_needed. A installed-agent SHA-256 matches the bundle. B transfer
837117f8dc6d152faf9a59c789a30cf1 reports UPDATED at the exact g412 revision.
Neither installation needed a retry, manual session stop, UI kill or reconnect.
The initial receipt observation printed no new record; direct inspection later
confirmed terminal success, and the update was not restarted. The g413 tooling
audit found the monitoring script reparsed JSON DateTime values with a local
offset, so this poll must not be used as installation-timing evidence.

Collection 3a6f4ff1236b50f16d17bc5d4eb7a08f at 02:10:11Z contains 28 current-build
events on each device, CONNECTED on both, service/input READY, and no current-build
fault/startup-failed event. A doctor independently reports healthy CONNECTED,
matched CLI/UI/resident/installed revisions, verified launch record and completed
stage cleanup. This normal installation does not identify g410's original handle
owner, prove every future installer transition, or establish physical input
failure/cleanup acceptance.

Final remote inspection has no open PRs and still retains Dashboard #1 plus
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70. No issue was closed from adapter tests or
idle health alone. Missing acceptance is tabulated in
g410-geometry-invalidation.md. The attended physical matrix availability question
is still pending; pair identity/signing-service availability remain unknown.
The local certificate stores previously inspected had no usable code-signing
certificate. No further implementation unit was started past ten minutes.

Next: actual attended held-key/Winlogon recovery and the remaining production
native boundaries under #50/#59, plus exact lock-owner attribution if a future
installation genuinely fails. Do not provoke another healthy installation only
to search for a handle. Keep native desktop checks and first-click shield
semantics intact. Only local main was modified; no push or remote ref changes.
