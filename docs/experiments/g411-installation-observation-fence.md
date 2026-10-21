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
