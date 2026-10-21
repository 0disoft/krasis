# g402: deny presence checks during proof retirement (#50/#59)

The production input client's full read-only authority query is context-bounded,
but its following pinned-presence check used a blocking authority read lock.
A concurrent Close waiting for a stalled native reader can therefore also hold
that presence caller behind the writer. The input client invokes this check
while holding its serial wire gate, after the full query and before submission.
Source keepalive and release-child service-presence checks shared the same path.

The presence check now uses TryRLock, matching the physical source authority
admission rule. A closing or busy proof returns ErrAuthority without touching
handles. Available proofs retain their fresh native pipe/process check; the
reader still pins handles until it returns. No cached authority, successful
keepalive, retry, early handle close or input/release capability is added.
The native presence query itself and Close's obligation to join readers remain.

Inputworker, residentagent, peer and application suites and relevant vet pass.
The regression failed before the fix because a heartbeat presence check waited
for the held retirement gate. It covers ordinary/source presence rejection,
fresh checks after gate release and rejection after actual Close. Existing
overlapping native reader and Close-joins-readers tests remain applicable.
This is a deterministic ownership test, not a reproduced Windows syscall hang.

The final #50 call-graph audit also identifies direct protected-input inspection
in the portable resident's press/admission path and remaining native presence
queries. These are not solved by moving SendInput into children. Keep #50 open
until its bounded-dispatch and cleanup claims cover those production paths.
#59 still has physical/queue semantic gates and native desktop/cursor callback
work. No API/DB, dependency, runner, firewall, signing, remote ref or issue-comment
changes. Generation 402 identifies the authorized A/B application bundle.

## Paired application and remaining work

Both PCs applied g402 revision 9c77f02ab78ae561b1c6daf8c2aeaedb34e64299.
UI 85 tests, native builds, installer provenance and eight-file package checks
pass; offline verification accepted 28,126,816 bytes. A installer exited 0,
installed agent SHA-256 matches, and the installed-input cursor/same-point
placement and recovery-shutdown smoke exited 0. Recovery entrypoints report
recovery_ready. B update receipt 4a4d6b623cb7298790eeb03bb226f737 reports UPDATED;
its installation receipt is complete/result 0 with no recovery required.

A returned to LOCAL after B replacement and was explicitly reconnected.
Collection f8f320b04b13984f14834eb7aa266fa8 captures the transition, including
B's acquisition timeouts before A's listener was restored. Follow-up collection
7b4be78ac877ea8da279104824c6f32c at 00:23:08Z/09Z has 69 A / 44 B current-build
events, READY services, protected source capture readiness and no fault or
startup-failed code. Both logs' latest connection state is CONNECTED; A's older
recovery receipt still says LOCAL and is not used as a live status observation.
A's fresh doctor independently reports healthy/CONNECTED and matching CLI, UI,
resident and installed revisions with completed stage cleanup.

No actual held-key, Winlogon, power, DHCP, home-pair, accessibility/DPI or signed
release acceptance is inferred. Race detection was not run with CGO disabled.
The current environment exposes no KRASIS signing configuration or current-user
code-signing certificate. Device-pair identity and availability of a real signer
were requested from the user; their absence is not a completed hardware/signing
gate. No extra issue was closed, comment posted, branch/worktree created or
remote Git ref changed. Both implementation units in this continuation started
before its ten-minute cutoff; no further unit was begun afterwards.

Next: audit/bound the portable resident's direct protected-input inspection and
remaining presence ownership before closing #50; continue #59's physical and
producer gates and native callback dependencies. The implementation of #16's
full-app setup/removal already has historical g240 evidence in
results/2026-10-08-app-setup-smoke.md, although its GitHub description is stale;
signing, current-revision lifecycle/data/firewall acceptance remain separate.
The open issue set remains #4/#5/#6/#7/#15/#16/#20/#50/#59/#70.
