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
