# g396: keep live native checks out of isolated journal owners (#50)

The installed input worker retained its journal outside the native child but
still performed synchronous SCM/WTS authorization and desktop checks around
every child command. A slow parent check could therefore stall the same journal
before reaching the bounded native IPC. The before-login journal similarly
repeated desktop checks even though its fixed native child owns Winlogon input.

Only the concrete isolatedInput transport now executes live journal operations
without that second parent native transaction. Its independently authenticated
child still performs fresh authority, parent-lifetime, lease/grant and desktop
checks before native commands. No cached authorization, generic caller-selected
bypass, new wire option or new native capability is introduced. Direct native
ports keep their existing executor. Parent cursor observation no longer adds a
second desktop scope around the child's own observation.

Initial parent authentication/desktop readiness remains unchanged. On cleanup,
the native child must still fully retire before parent key-up. Recovery retains
the original parent authority/desktop executor; a rejected recovery keeps held
state and ErrRecovery. Its synchronous native calls remain a separate #50 gap,
so this does not complete or close that issue.

The parent reports only measured IPC/apply duration; native authority/scope and
desktop fields are unknown rather than invented from the journal's desktop.
Native failure certainty, partial counts, operation order and pressed ledger
semantics are preserved without retry.

## Verification

Inputworker tests pass, including the real hidden native stand-in which accepts
a simulated down and then stalls on Move. Its journal now uses an executor whose
parent native-check path always rejects, yet input reaches the child and held
input is released only after exact child exit. Additional tests preserve child
failure identity/certainty, reject cancelled/missing/retired transports before
dispatch, retain direct-port checks, and prove login recovery still invokes the
parent executor and preserves Remaining on rejection. No native keys/buttons
are injected by these fixtures.

The preceding g395 paired collection f5186ea97de52c7008b1e91ef4289ade confirms
both PCs CONNECTED with installed_source_capture_protected_ready. A doctor is
healthy with matching CLI/UI/resident/installed g395 builds. The earlier pending
reconnect is resolved. No physical input acceptance is inferred from this.

Next #50 work is the recovery owner's synchronous authority/desktop path and
one-time startup/cleanup calls; #59 callback dependencies and #70 actual endpoint
change remain separate. Hardware, UI/accessibility and signed release gates
retain their own outstanding conditions. No issue comments or push. No public
API, DB, dependency, runner, firewall, signing, branch or worktree changes.

## Packaged application and recurring startup failure

Inputworker, residentagent, application, peer, input-command and agent-command
test/build checks pass, as does ordinary vet for those packages. UI's 85 tests,
native builds, installer provenance and eight-file archive checks also pass.
The verified update archive is 28,118,774 bytes.

Both PCs applied g396, revision 7d354c880681df70e52880eb6c3c82b4cf2297e8.
A installer exited 0, installed agent SHA-256 matched, and the actual protected
service/native-child cursor read/same-point placement/clean-shutdown smoke
exited 0. Only the session-0 input service remained after the smoke. This tests
the real installed ordinary dispatch path changed here; it injects no native
key/button and does not test before-login input. B receipt
d8ec8e0bafc58ac4879170e5d2d23b57 reports UPDATED at the same revision.

A's first source startup failed at 23:31:05.7757363Z, worker_ready, with the new
fixed category installed_source_startup_eof. Source-hook triggered the session
failure. The service summary contained no source report for this attempt, so
failure before source capture/report creation is possible but not proved.
This repeated startup failure must not be erased by the later successful smoke
or described as fixed by removing journal checks. Preserve collection
3c28603f5d836075f1301bc9ed68d54f and the initial local source events.

A was safely stopped and its same installed g396 agent started again, followed
by explicit Connect B. No Disconnect command was used during replacement.
Collection f16a2979876c7f9fee461c17bc0f5f91 at 23:33:02/05Z records both PCs
CONNECTED at 23:32:53Z and protected capture ready at 23:32:54Z (B) and
23:32:55Z (A), with READY services. A doctor reports healthy with matching
CLI/UI/resident/installed builds. This short recovered window is not a
fault-free stability run. Ten issues remain open.

First next priority is the recurring source startup EOF, particularly admission
and pre-capture worker bootstrap failures that currently leave no SourceExit.
Then finish #50 recovery-side native ownership and #59 callback work. Physical
held-input, lifecycle fault matrix, home pair, endpoint change, accessibility,
soak and signed-release acceptance were not run in this unit. No new independent
implementation was started after the ten-minute cutoff. Source and application
evidence remain local main commits; no push or issue comments.
