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
