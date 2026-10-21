# g400: isolate installed login release recovery (#50)

The installed Winlogon journal now prestarts independently authenticated input
and release-only children before admitting a connection. Its previous recovery
path entered a native desktop/authority scope in the journal process; a native
stall there could prevent a final pressed-state receipt. Both live dispatch and
recovery now use child IPC, retaining the authoritative journal in the parent.

The new fixed --native-login-release-worker mode uses the same immutable login
bootstrap, protected installation, genuine service and exact live parent checks
as the input child. It accepts only heartbeat and key/button-up frames through
the existing release-only decoder. Its guard has no independent native release
capability. Every operation still checks the original active Winlogon session
and live grant/profile; recovery does not gain permission on Default, after
logon or after grant revocation. The exact authenticated service process is
pinned before admission, so StopPending alone does not substitute a foreign or
dead service. No caller-selected desktop, executable or recovery target exists.

The login job now permits exactly three processes under kill-on-close: journal,
input and release. Exact input-child retirement precedes release; exact release
retirement is required even for an empty journal or failed input retirement.
Unacknowledged releases preserve Remaining/ErrRecovery, without replay or local
native fallback. Startup failure of the release child retires the input child.
The outer owner also cleans both transports if cancellation prevents the journal
from starting. Native source-hook/startup and direct development-only input
paths retain their existing ownership and require separate final audit.

## Verification

Inspection also found that RunWinlogon itself authorizes initial binding, while
Scope.Do checks thread/desktop activity. The installed native login input child
now explicitly rechecks service, parent and live grant before every command,
as does the new release child; it no longer relies on startup authorization.

Inputworker, input-command, loginagent, inputdesktop, residentagent, application,
peer and localapi suites pass; agent compilation and relevant vet pass. Hidden real
subprocess fixtures exercise the login journal with successful release and a
release child that ignores cancellation. They require both exact process exits,
preserved held state for unacknowledged release, and zero native parent executor
calls. Empty-journal and failed input-retirement fixtures reject missing release
retirement without retry. Windows tests verify protected-mode rejection and the
three-process login bound. No test injects real native keys/buttons.

This is implementation evidence, not physical Winlogon acceptance. #50 remains
open for the final call-graph audit and device fault acceptance. #59 callback
dependencies and #70 physical endpoint change remain separate, as do the
hardware, UI and signed-distribution gates. No API/DB, runner, dependency,
firewall, signing, issue-comment, branch/worktree or push changes are included.
Generation 400 identifies the new executable bundle for the authorized A/B
application. Applied state and further validation are recorded after completion.

## Paired application and next work

Both PCs applied g400 revision b4421dc6d5ff03b264a1c32c1cccb0f42a8ce67d.
UI 85 tests, native build, embedded installer provenance and eight-file package
checks pass. Offline verification accepted 28,125,278 bytes. B update receipt
5f05932e8fcf554b7226bf06f16ffabc reports UPDATED at the same revision.

A's installer receipt at 00:08:59Z is complete/result 0 with no recovery needed;
its installed agent SHA-256 matches the packaged agent. The installed native
cursor/same-point placement and recovery-shutdown smoke exited 0. The immediate
process snapshot still contained the retiring journal; the next snapshot before
reconnection contained only the session-0 service. No orphan native input or
release child was retained. The authorized UI was restarted from g400 and its
recovery entrypoints report recovery_ready.

B's installer receipt at 00:09:47Z is also complete/result 0. Collection
8309db8f24e5d42966ae44ff9f01ae39 captured A still CONNECTING during replacement;
do not mislabel that early snapshot as both connected. Follow-up collection
adafea74723c5d54978cafc4b34e3866 at 00:10:42Z/44Z confirms both CONNECTED,
both services READY and protected capture ready. Its 33 A / 36 B current-build
events contain no fault or startup-failed code. Doctor confirms matched CLI,
UI, resident and installed builds and completed stage cleanup.

These ordinary-desktop checks do not exercise the new Winlogon release mode,
held native key/button recovery, lock/sleep, physical endpoint change, home
hardware, DPI/accessibility or signed external release. The g377 user report is
not reused as g400 evidence. No additional GitHub issue was closed; the ten
remaining issues are #4/#5/#6/#7/#15/#16/#20/#50/#59/#70. Only local main and its
primary checkout exist. No issue comments or remote refs were changed.

Next priority is #50's complete production call-graph audit and an attended
held-input/Winlogon failure trial, followed by #59's physical/queue semantic
gates and synchronous desktop/cursor callback dependencies. The ordinary
portable product route already uses the paired input/guardian transport;
RunLoginInput's direct native helper is referenced only by its own test. The
installed ordinary and login product routes now select isolated journals.
Their source-capture owners still require separate #59 work. No second
implementation unit was started in this continuation.
