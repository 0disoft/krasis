# Linux desktop VM lifecycle validation — 2026-11-11

## Environment and scope

Two independent Debian 13 QEMU/TCG guests communicated through forwarded TCP
ports using the actual agent and CLI, PIN pairing and mTLS. A used a dedicated
UID/PAM/logind session with Xvfb. B used a different UID, LightDM/Xfce, Xorg's
modesetting driver and the Bochs DRM kernel module. No X11 authority was shared.
B used kernel `6.12.116+deb13-amd64`. Test account credentials were disposable;
they and the VM disks/screenshots are excluded from Git.

The graphical guest used an explicit XDG autostart entry for `krasis-cli watch`.
The fixture auto-login only made the reboot test unattended; this is not a
recommendation to enable auto-login or remote input before authentication.

## Observed defects and corrections

1. Without watch, a real LightDM lock stopped the agent and cleared received
   held input. PAM authentication restored the **same** graphical session but
   left the agent absent. The opt-in watcher restores the saved connection
   after a fresh safe-session check, without re-pairing.
2. Xfce ScreenSaver 4.18.4 exposed `GetActive=true` while logind still reported
   `Active=yes`, `LockedHint=no`. A logind-only watcher could therefore restart
   input during the lock. The agent and watcher now also fence on the current
   user's authenticated Unix desktop bus, active locker state, ownership changes,
   missing/unreadable locker state and bus loss.
3. A locker can stall while acquiring an input grab before broadcasting its
   active signal. The existing 15-second authoritative check now also queries
   the pinned locker, with a two-second budget per call. A timeout revokes input;
   it is never interpreted as an unlocked desktop.

Initial cloud-kernel/VESA trials crashed Xorg during VT switching. A later
LightDM GTK greeter also exited with `BadWindow`. Those trials were excluded
from successful lifecycle results; the successful PAM UI tests used Xfce's
screensaver on the full DRM kernel. Test automation disabled AccessX/StickyKeys
and automatic idle activation; explicit locking and lock-on-suspend remained on.

## VM results

| Scenario | Evidence/result |
| --- | --- |
| Actual graphical lock UI | LightDM greeter and Xfce PAM unlock dialog observed with QMP screenshots; no synthetic logind-only replacement |
| Held input at lock | A entered B via the native boundary and held a key/button. `loginctl lock-session` stopped B; B's held state cleared and A could acquire/release its X11 grab **before** the source key/button were released |
| Locked startup/resume | `GetActive=true` prevented agent startup despite `LockedHint=no`; the watcher stayed alive |
| PAM unlock | Keyboard entry of the disposable password changed `GetActive` to false; both agents returned to `CONNECTED` without another pair/connect command |
| Real S3 suspend | `/sys/power/mem_sleep` selected `deep`; kernel logged `PM: suspend entry (deep)` and QMP returned `status=suspended`, `running=false` |
| Resume safety | QMP `system_wakeup` produced kernel `ACPI: PM: Waking up from system sleep state S3` and `PM: suspend exit`; boot ID stayed unchanged, received held input cleared and the locked agent stayed absent |
| Unlock after S3 | PAM password entry after resume restored both `CONNECTED` states without a new connection command |
| Actual reboot | Guest boot ID changed; graphical login and XDG watch restored `CONNECTED` on both guests, with the same paired fingerprints and no new pair/connect command |
| Manual stop | Explicit `disconnect` kept B `LOCAL` while the watcher continued polling; explicit new connection commands on the peers restored the fixture |

The S3 entry was observed at 12:44:08 UTC and resume at 12:44:44 UTC. Reboot
changed boot ID from `6028545b-c014-4739-b7ff-dad204989ebb` to
`5ec016e2-d3c9-4335-9122-2213af1f0c0d`; S3 preserved the latter ID.
The final locker-fenced build was rebooted again to boot ID
`97e40d37-1cf4-4377-8fa2-b33762ec1e05`. Both agents were observed `CONNECTED`
at 12:54:20 UTC with the original paired fingerprints, without another connect
command.

## Repeatable checks

Use disposable guests and accounts, with the real PAM/X11 session environment.
Never apply synthetic input or suspend/reboot commands to a working desktop.

1. Pair and save a connection normally. Start `krasis-cli watch` from the actual
   graphical login session, preserving its `DBUS_SESSION_BUS_ADDRESS`.
2. Enter the peer through its native screen boundary. Hold a key/button at the
   source, lock the target with `loginctl lock-session SESSION`, then assert API
   shutdown, received input release and source grab release.
3. Query the real locker's `GetActive`, authenticate through its visible PAM UI,
   and assert the same paired identities become connected without reconnect CLI.
4. Run `systemctl suspend --no-block`, assert QMP's **suspended** state, then wake
   with QMP `system_wakeup`. Check kernel S3 logs and an unchanged boot ID. Assert
   no input agent while locked, then authenticate and check connection recovery.
5. Reboot the guest and verify graphical-session autostart, unchanged identity
   fingerprints and automatic connection. An XDG entry must run only after login.

Native Linux unit executables covered launcher/watch, singleton ownership and
logind/locker monitoring. Related Windows package tests and Linux cross-vet
passed. CI retains Windows and the existing Linux job; launcher, lifecycle and
singleton race tests share the existing X11 race invocation.
The headless Xvfb/PAM fixture explicitly removes a possible PAM-exported session
bus variable. Its real logind session identity is preserved; desktop locker
behavior is covered separately by this VM validation, not a fake locker.
The additional `TestLinuxX11ActualCLIInteroperability`/`ActualLifecycle` rerun
on the TCG guest did not pass: cold PAM startup exceeded the fixture's eight-
second readiness window, and a subsequent boundary check encountered X11 I/O
timeouts. These failures are not counted as successful desktop lifecycle
evidence. The existing native Ubuntu CI job exercises the same fixture change;
no product or test deadlines were increased to hide these failures.

## Limits

This is VM functional evidence, not physical suspend, driver/power-loss, hardware
latency, Windows-to-Linux hardware or Wayland validation. The desktop watcher
requires a supported ScreenSaver service on a same-UID Unix session bus; an
unavailable or unsupported desktop is blocked rather than assumed unlocked.
Locker signals are the immediate stop path. If a locker delays or omits its
signal, the fallback snapshot runs every 15 seconds, with a two-second budget
per locker query; this is not an instantaneous lock-detection guarantee.
Plain headless X11/PAM agents retain logind-only monitoring and cannot use watch
without a graphical session bus. HTTP API, DB schema and Windows runtime policy
were not changed.
