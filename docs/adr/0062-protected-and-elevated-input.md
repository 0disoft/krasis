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

## Acceptance sequence

1. Build a one-shot, destination-local probe with no peer/network input path.
   It sends only a bounded Escape press/release to a positively identified
   foreground target. Test an elevated PowerShell window and the observed
   `CredentialUIBroker.exe` dialog separately. Report only result codes, never
   window text, commands, PINs, clipboard content, or key payloads.
2. On B, verify the visible effect for both target classes. If the PIN prompt
   does not respond, keep the runtime guard and try the driver path only after
   its signing, installation, rollback, and device access boundary are ready.
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
