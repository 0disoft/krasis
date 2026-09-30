# ADR 0062: Validate a separate Windows input path for elevated and credential UI

- Status: Experimental; product support is not yet accepted
- Owner: Krasis maintainers
- Scope: destination PC's current interactive user session

## Problem and evidence

Krasis injects destination input with `SendInput` from an unelevated resident
agent. On the gram13 destination, a Windows Security PIN dialog owned by
`CredentialUIBroker.exe` caused `protected_input_detected` and a clean
disconnect. The attended generation-60 elevated probe positively observed the
broker and sent an Escape press/release, but returned `dialog_still_open`. The
operator confirmed that the destination's built-in keyboard Escape closed the
same dialog. The operator also reports that A's mouse and keyboard cannot
operate an elevated PowerShell window on B. These are distinct target classes;
the probe does not establish that any proposed replacement API works for them.

`SendInput` is subject to UIPI. Windows can reject synthetic input without a
useful UIPI-specific error. Windows security updates beginning in January 2026
also restrict interaction with credential dialogs. Do not remove the existing
protected-input disconnect merely because a new API accepts a call.

On the gram13, the packaged `inputInjectionBrokered` probe sent Escape but the
PIN dialog remained open; its elevated PowerShell result did not clear the
operator's test input either. The operator subsequently confirmed that Deskflow
can type into the same Chrome-triggered PIN dialog. Deskflow's Windows daemon
launches its input process into the interactive session using a SYSTEM token
with UIAccess and switches its input thread to the active desktop. This is
evidence for testing that process boundary, not permission to copy its daemon
or expose an unauthenticated privileged IPC endpoint.

## Decision

Expand the product goal to include attended control of higher-integrity apps
and the destination user's Windows Security PIN dialog, but change the runtime
only after a separate destination-local capability test demonstrates delivered
input. Test Microsoft's packaged `InputInjector` with its
`inputInjectionBrokered` restricted capability first. It is documented for
remote access and input to administrator applications; that documentation does
not prove this PIN dialog accepts its events. Do not infer success from a
non-null injector, an API success code, or a vanished foreground window alone.
Require an operator-observed effect on B.

If the packaged API fails on the credential dialog, investigate a signed VHF
virtual HID source driver as a separate decision. VHF represents a device class
to Windows, but neither Microsoft documentation nor this ADR proves its reports
will be accepted by the affected PIN dialog. Do not disable Secure Boot, turn
on test signing on the everyday laptops, install a test driver, or silently
trust a development signing certificate as a workaround.

Before a driver, test the narrower Windows UIAccess route with the independent
`experiments/uiaccess-input` probe. The probe's EXE must be signed, trusted and
installed under an administrator-owned `Program Files` directory; it verifies
the granted UIAccess token before sending one Escape pair. The developer test
certificate is never installed automatically. Generations 91 through 94 carry the
signed experiment files in a peer update after the receiver reaches generation
90. The update stages these files beside the updated UI; it never launches the
probe or trusts its certificate. This stage has no peer input or resident
helper. If B confirms the same PIN dialog closes, design a separately
installed input-only helper with authenticated, exact-lease IPC and no arbitrary
command execution. Preserve the ordinary agent as unelevated. A failed probe
keeps the existing guard; a successful API return alone is inconclusive.

The attended g94 UIAccess test on GRAM13 had the expected signed executable,
trusted certificate and `TokenUIAccess` grant. It observed the actual Chrome
Windows Security PIN dialog and `SendInput` accepted the Escape pair, but
returned `dialog_still_open`. The operator closed the dialog with B's touchpad;
A's remote mouse and keyboard did not work while it was open. The test EXE and
certificate were removed and checked absent. This disproves that the tested
scan-code-only, marked UIAccess input path solves this dialog.

The attended g97 comparison also failed on GRAM13. The operator confirmed
`dialog_still_open` in the retained result window, process exit code 5, and the
PIN dialog remaining open immediately after the trial. Only after recording
the result did the operator close the PIN dialog with B's touchpad. Removal
returned 0; the installed directory, executable and experiment certificate in
Root and TrustedPublisher were checked absent. Using `VK_ESCAPE` plus scan code
without an extra-info marker therefore did not fix the tested path. This does
not distinguish active-desktop attachment from the SYSTEM/session token
boundary, or establish that all `SendInput` configurations fail.

Generation 98 isolates active-desktop attachment with a fresh, windowless input thread.
Keep g97's Escape event shape; open the current input desktop, require Default,
bind the worker with `SetThreadDesktop`, and revalidate the exact foreground
broker and session before the single pair. Report binding success and whether
the worker was already on Default, without recording desktop names or input
content. Observe visibility of the original dialog after injection rather than
treating a foreground change as dismissal. An open/bind/target failure sends no
input. A failed trial keeps the product guard. If B still confirms failure,
investigate a separately installed SYSTEM input process with active-desktop
ownership rather than broadening the ordinary agent or weakening the PIN guard.
The generation-98 update carries the pinned, separately test-signed probe and
its installation/removal scripts beside the product executables. Delivery does
not install the probe, trust its certificate or run the trial.

The attended g98 trial on GRAM13 returned `escape_send_failed`; the B tester
directly confirmed process exit code 4. The operator reported successful
input-desktop binding, the worker already being on the same desktop, and the
PIN dialog not closing after the trial. The six files, EXE hash, certificate
thumbprint, signer and expiry matched the independently supplied values;
installation returned 0, the installed signature was Valid, and execution was
unelevated. The operator manually closed the PIN dialog, but whether this
happened after all result items were recorded is unknown. Removal returned 0;
the installed directory, executable, Root/TrustedPublisher certificate and
trial process were checked absent.

This rules out desktop binding alone as the fix for the tested path. The g98
code returns 4 when the primary `SendInput` call accepts fewer than two events;
it did not retain the accepted count or last error. Do not infer zero accepted
events, a particular Windows error, or a proven UIPI cause from this code. The
next probe records the primary accepted count and immediate `GetLastError`
before cleanup, plus the cleanup key-up accepted count. Even these values do
not identify UIPI blocking, and API acceptance still does not prove delivery.

## Next bounded SYSTEM trial

Desktop attachment no longer warrants another ordinary-user UIAccess-only
trial. Deskflow's elevated watchdog branch duplicates the token of the active
session's `winlogon.exe`, sets TokenUIAccess and launches its input process.
Its unelevated branch uses the user's token instead. Which branch was active
in the operator's successful Deskflow run is not yet established.

Prepare a separate, attended service-launch experiment before changing the
product. Its acceptance contract is:

- A locally approved administrator installation uses a pinned executable in
  an administrator-owned Program Files directory. No automatic trust or
  service registration occurs through the peer update.
- A demand-start, one-shot service launches only its fixed input probe into
  the locally selected, current console session. No remote input, arbitrary
  executable, command, PID, token source or session selector is accepted.
- Verify SYSTEM identity, expected session, granted UIAccess and Default
  desktop in the child before the single Escape pair; recheck session and
  foreground broker immediately before input. Other desktops remain excluded.
- Do not turn the service itself into an interactive GUI. Keep the privileged
  child's result to numeric API metadata and fixed result codes; no network,
  clipboard, PIN, window text or screen capture is part of the experiment.
- Bound target observation and process lifetime. Service stop, session change
  and timeout terminate only the child handle created by this trial, without
  restarting it or executing queued input later. Installation and removal
  must verify exact ownership, reject existing conflicting services and files,
  and confirm that the child and service are gone before deleting files.
- Include primary input acceptance, last error and cleanup acceptance in the
  report, together with the operator's actual PIN response. A local Escape
  success is a prerequisite, not proof of A-to-B elevated/PIN input support.

`experiments/system-input` implements this local capability trial. The service
duplicates its own LocalSystem primary token, binds it to the installed
operator's console session and sets UIAccess. This avoids opening winlogon
but is not asserted to be identical to Deskflow's elevated token configuration.
The operator SID and session are recorded only in the administrator-owned
installation configuration and rechecked through WTS before launch and input.
The service and windowless worker share one fixed executable. The child joins
a kill-on-close Job atomically at process creation, before its suspended thread
is resumed. A per-install CREATE_NEW claim prevents repeated starts.
Only numeric, content-free evidence is written to fixed files. The installation
is demand-start with SYSTEM/administrator service access, and checks the pinned
hash and certificate before local trust and registration. Removal verifies
service executable/account, installation hashes and process absence.
The g99 installation on B passed artifact/signature checks, returned 0 and
produced a Valid installed signature. Service start failed with exit 1 and
Win32 740 (ERROR_ELEVATION_REQUIRED). After 130 seconds the run claim and both
result files were absent, with zero trial processes. This is a service-bootstrap
failure, not evidence about SYSTEM Escape delivery. The operator manually
closed the PIN dialog; trial effect and manual-close ordering are unknown.
Removal returned 0; service absence (1060), zero processes, installation files
and Root/TrustedPublisher certificate absence were confirmed.

The shared EXE's manifest incorrectly requested uiAccess=true, even for SCM
bootstrap. This is the leading explanation for the pre-entrypoint 740 failure;
the error alone does not establish the exact loader policy on B. Change the
bootstrap manifest to asInvoker/uiAccess=false. The service continues explicitly
setting TokenUIAccess on the duplicated child token before CreateProcessAsUser;
the child still refuses injection unless its actual token grants UIAccess.
The builder checks the embedded manifest, and a local regression launches the
uninstalled cache EXE through ordinary CreateProcess-style APIs and requires
code 12 for service/worker/invalid invocations, with no service or input execution.
This verifies loader entry and denial locally; actual SCM start, child UIAccess
and delivered Escape remain pending on B. Keep the resident product unelevated
and its guard active until attended evidence is obtained.
Generation 99 carries the signed SYSTEM trial in the same atomic six-file
payload accepted by g90+ receivers. The legacy test EXE filename is retained
for transport compatibility; its system-input README and scripts select the
new installation/attended execution flow. The update only stages these files.
Generation 100 carries the startup-corrected manifest, independently pinned
signed EXE and current trial instructions through that same six-file transport.
Local process creation/refusal and manifest checks passed. On B, g100 artifact
and signing checks passed, installation returned 0 and the installed signature
was Valid. Service start returned 0, with Running state and start error 0.
The child result was code 4, win32Error 5, SYSTEM=true, consoleSession=true,
desktopBound=true, alreadyDefault=true and inputAttempted=true. SendInput
accepted zero of two events; cleanup accepted zero releases. The child's
UIAccess gate precedes target observation and input, so reaching this result
also establishes that its actual TokenUIAccess check passed. The service
propagated child code 4 with launch error 0; its desktop/input fields describe
the service, not a second input attempt. The PIN dialog remained open until
the operator manually closed it after inspecting both JSON files. Removal
returned 0, with service absence (1060), zero trial processes and absence of
installation files and the exact Root/TrustedPublisher certificate confirmed.

This establishes refusal of this SYSTEM/UIAccess/default-desktop SendInput
path. Error 5 alone does not identify UIPI or the specific rejecting policy.
Deskflow's elevated watchdog duplicates the current session's winlogon token;
this experiment instead duplicated the service token and changed its session.
Token provenance remains an untested difference. The next bounded comparison
must select only the verified System32 winlogon in the installed console
session, validate its SYSTEM identity and token session, and retain all existing
operator, target, one-shot, lifetime and job-ownership checks. It must not accept
an arbitrary token source or expose a remote privileged input endpoint.

The comparison selects exactly one System32 winlogon process in the expected
active console session, rechecks its image through the opened process handle,
and validates SYSTEM identity and TokenSessionId before duplication. It grants
UIAccess without rewriting the source session. Failure has no service-token
fallback. The worker validates its actual UIAccess and token/process session
agreement before target observation. Added numeric/boolean evidence makes the
worker's token state explicit and distinguishes successful winlogon-token child
creation in the service report from the worker's actual injection result.
Only the temporary service enables debug privilege for source-token access;
the normal product remains unchanged. Existing ownership, fixed executable,
operator SID/session, foreground target and lifetime restrictions still apply.
Local x64 compilation, bootstrap manifest extraction and process-creation
refusal checks passed for uninstalled service, worker and invalid invocations
(code 12, no service or input). B's attended token comparison remains pending.
Generation 101 stages the independently pinned, test-signed comparison through
the existing six-file update transport. Receiving the update neither trusts the
certificate nor installs or starts the service; B's local attended procedure
is still required. Its actual token launch and PIN response are unverified.

## Acceptance sequence

1. Build a one-shot, destination-local probe with no peer/network input path.
   It sends only a bounded Escape press/release to a positively identified
   foreground target. Test an elevated PowerShell window and the observed
   `CredentialUIBroker.exe` dialog separately. Report only result codes, never
   window text, commands, PINs, clipboard content, or key payloads.
2. On B, verify the visible effect for both target classes. If the PIN prompt
   does not respond, keep the runtime guard and follow the desktop-bound worker,
   separate SYSTEM process, then driver investigation sequence. Each needs its
   own installation, rollback and input-boundary evidence before integration.
3. Integrate a proven input backend behind the existing destination adapter.
   Keep the normal agent unelevated. A local helper must accept input only for
   the exact authenticated lease, session, epoch, and sequence; it must not
   expose a general-purpose elevated input service to local or remote callers.
4. Recheck pressed-key/button release, local emergency takeover, session loss,
   helper crash, screen lock, UAC secure desktop, update, and uninstall. A
   backend switch cannot replay input whose delivery is uncertain.
5. On two physical PCs, verify A-to-B mouse movement and click, typing in an
   elevated PowerShell, text copy/paste in both directions with clipboard
   sharing enabled, and attended PIN entry without disclosing or recording the
   PIN. Verify both PCs recover local input after every interruption.

The existing opt-in clipboard transport needs no privilege escalation merely
to transfer text. Paste into an elevated target still depends on the proven
destination keyboard path; copying from an elevated target also needs physical
confirmation of clipboard observation. Do not claim clipboard acceptance from
an API-level input test alone.

## Current boundary and rollback

Until acceptance passes, the resident agent keeps its protected-input guard
and release documentation must not promise elevated or PIN-window control.
The probe is not shipped as an automatically installed privileged component.
Rolling back an experiment removes its separate package or helper; the
existing user-mode agent and local-input recovery remain the fallback.

## Primary references

- Microsoft `SendInput`: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput
- Microsoft input injection: https://learn.microsoft.com/en-us/windows/uwp/ui-input/input-injection
- Microsoft credential dialog change: https://learn.microsoft.com/en-us/troubleshoot/power-platform/power-automate/desktop-flows/windows-credentials-dialog-issues
- Microsoft VHF: https://learn.microsoft.com/en-us/windows-hardware/drivers/hid/virtual-hid-framework--vhf-
- Microsoft UIAccess policy: https://learn.microsoft.com/en-us/windows/security/application-security/application-control/user-account-control/settings-and-configuration
- Microsoft input desktop: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-openinputdesktop
- Microsoft thread desktop: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setthreaddesktop
- Deskflow input-desktop worker: https://github.com/deskflow/deskflow/blob/master/src/lib/platform/MSWindowsDesks.cpp
- Deskflow interactive-session watchdog: https://github.com/deskflow/deskflow/blob/master/src/lib/platform/MSWindowsWatchdog.cpp
- Deskflow privileged IPC advisory: https://github.com/deskflow/deskflow/security/advisories/GHSA-6rx5-g478-775c
