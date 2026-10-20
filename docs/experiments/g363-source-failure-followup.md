# g363-g365: input recovery trial and installer repair admission

## Baseline and scope

On 2026-10-20 the operator authorized investigation, a new paired trial build,
application, and an attended lock/PIN/login and typing trial. No remote Git
push, public release, reboot, or trust-policy change is authorized.

Before application A still runs g362 (1857565), connected to B, but its
KrasisInput service is STOPPED and doctor reports input_service_stopped with
PORTABLE input. B is ONLINE on g362 and allows the paired development update.
Collection 38c7dafda61bbea5599d5f5d6a18d463 preserves both sides before changes.
The original A runtime receipt and rotated agent log are additionally preserved
under .git/krasis-improvement/g363-preapply, outside tracked sources.

A's service failed at 2026-10-20T08:13:19.1380446Z. Its final source report
records command=failed, reader=stopped, capture/sender/interrupt=joined,
retirement=failed_sent, report=recovery, operation=source-command-loop,
and no known Win32 error. A heartbeat response exceeded one second at
08:13:17.5752369Z; the subsequent stop was rejected at 08:13:18.9616882Z.
This establishes ordering, not the original Go command error. In particular,
the input-channel heartbeat fix is not proof of this source-channel cause.

The trial includes the committed changes since g362: bounded unsent-input
authorization, heartbeat progress during input authorization, preparation
frame validation, pipe-close diagnostics, noninterfering failure observers,
pre-native source activation rejection classification, and command identity
in final reports. Native cleanup and unknown-command recovery fences remain.

## Acceptance and recovery

Use the existing generation-scoped workspace builder and offline update
verification, then send to B and wait for its exact generation/revision.
Apply the same bundle to A through normal single-instance replacement.
Confirm matching UI/agent/installed-agent metadata, running input service,
CONNECTED and PROTECTED/CONFIGURED, and fresh paired diagnostics.

Retain g362 and the saved failure evidence. If g363 fails, collect its first
failure before further replacement and safely stop the affected input session;
do not loop restarts, disable fencing, or silently roll back to the already
failed g362. Recovery must use a verified existing package and preserve trust.
Real lock/PIN/login, rapid returns and ordinary-window typing still require
the operator's observations; startup readiness alone is not stability proof.

No public API, DB, dependency, CI runner, or repository hygiene changes.

## Verified package

Source revision 251755e3814366f7ea54a7c470d52d63461d0a64, generation 363.
Inputworker, wininput, residentagent, logindiag, localapi and productcli tests
with a 90-second package timeout and their go vet -unsafeptr=false pass.
The workspace builder passes all 85 UI tests, native frontend/Go compilation,
installer provenance, archive transfer roundtrip, icons and eight-file checks.
Offline update verify returns valid/ok, size 27835305 bytes. This is a local
unsigned development bundle, using the existing paired-update mechanism.

Independent PowerShell SHA-256 checks:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 213d0914737003ab11ba1ea48c88ba40840d36ebdefa2da36b7a0f83a11e3978 |
| krasis-agent.exe | 0cc3fa95d4c517d876ee801e0be8143fdcb26b0f2df0b354df43c1456f981f2d |
| krasis-cli.exe | ecd2c6e4b60e166a60099ac5635ae9b4b3c1c8a287f61d9a9508d06c6b53fb01 |
| krasis-install.exe | 80ee9e1f04f212876c971f1cf4f6a09c7017f81a13d9c24f692985a6488e9887 |

B transfer f88649296fc0c84aea06bc9e696ab600 is queued. Application and
attended acceptance are not established by these package checks.

## g363 application exposed a separate repair defect

B transfer completed UPDATED at revision 251755e, generation 363. A also
replaced its UI/portable agent with that revision. Its installed g362 service
remained stopped, so the explicit UI repair action ran once after verified
agent exit. At 08:58:29Z the installer reported result=5, stage=snapshot,
operation=service_repair_start, Win32=1066. No repeated repair was attempted.

The installer holds INPUT-TRANSACTION.lock during its snapshot repair, while
the old service rejects a normal start with that lock held and no journal.
This is a separate, source-confirmed explanation for the repair refusal;
it does not establish the original 08:13 source command failure.

G365 supersedes the unapplied g364 package. It persists a matching journal before starting the verified old service,
using the existing installer-only start phase and SCM argument g362 understands.
It retains the exclusive lock, retired-process check and pinned old images.
Ordinary reboot startup cannot use this repair journal. Only after a verified
clean stop is that journal finished and normal image
replacement allowed. An uncertain start/stop keeps the journal; no restart
loop or receipt deletion bypasses cleanup verification. Startup policy and
service permissions are unchanged. Both real application and interrupted
repair behavior require validation before claiming recovery.

## Final g365 package

Revision 64c8b30863dbeb67fd9504a4c1a2db5469bc1e21, size 27837612 bytes.
Inputworker and installer tests/vet pass after the installer-only startup
restriction. The unchanged frontend and third-party notices reuse the verified
g364 build inputs; all 85 UI tests already passed. Native executables and
embedded installer are rebuilt at this revision. Offline verification, package
roundtrip, eight-file and icon checks pass; independent EXE SHA-256 checks:

| File | SHA-256 |
| --- | --- |
| krasis.exe | ef00f0d1728cd7e8eb44096a690be27dc4a4c8ac38fe4b439846c8c7ba29db8f |
| krasis-agent.exe | 4424bf5cd112f38776e15b8638b6cfa8c8946832afafcdcad591d55212daffa8 |
| krasis-cli.exe | 6b83173cb39717cadf32ef5a08e7f2593b2a3ea9b02743783fe714b074fd88e7 |
| krasis-install.exe | 6828119fa9063b4c7bbdc57afbc3e702486639ed0664a14f1de6c0a1ed799bed |

B transfer 86e33b30804ffe93fda989672ebc05d8 is queued. G364 was not applied;
its unused local artifact is retained. Real g365 service recovery, paired
readiness and attended acceptance remain to be observed.
