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
The initial additional `TestLinuxX11ActualCLIInteroperability`/`ActualLifecycle`
rerun on the TCG guest failed. Cold PAM startup exceeded API readiness and a
subsequent boundary check encountered X11 I/O timeouts. Those initial failures
are not counted as successful lifecycle evidence.

## TCG automatic regression follow-up

The automatic suite runs on the one-vCPU, 1-GiB TCG guest, with real executable
builds, two dedicated UIDs, separate X11 cookies and actual PAM/logind sessions.
Its original 8-minute suite, 110-second scenario and 8-second API readiness
limits remain in place.

The following causes were isolated rather than increasing those limits:

- Seven scenarios rebuilt the same agent and CLI inside their timed bodies.
  The script now builds both once before starting X servers and timed scenarios;
  each scenario copies executables into its own private unit namespace. Missing
  or non-executable prepared files fail rather than triggering a hidden rebuild.
- PAM/user-manager startup was charged to API readiness. Transient units now
  use `Type=exec`, with an independent 8-second PAM/exec check before API readiness.
  Both checks stay within the original scenario budget. Two neutral real PAM
  login fixtures keep the logged-in users' managers alive between scenarios;
  the fixtures have no input, Xauthority or synthetic lock state and change no
  linger settings.
- Repeated sudo/PAM admission competed with the agents during CLI polling.
  The actual-suite runner is elevated once; each CLI drops to its target UID,
  GID and supplementary groups directly. Endpoint presence gates the initial
  probe, but only a successful authenticated CLI response proves readiness.
  Every polling callback receives the remaining bounded context, so late
  responses cannot become successful deadline checks.
- Re-entry assumed an inactive capture sampled a center warp within 30 ms.
  Both entries now use the same observed native center-to-edge movement flow;
  retries require the product's existing `boundary_dropped_not_ready` counter
  to advance with no new admitted/written request. Two position reads alone
  cannot exclude activation between them: a retry would then become spurious
  remote motion. An in-progress request is observed without additional warps.
  Entry also requires the real remote echo of a single harmless source pointer
  movement. The cursor is centered before capture's physical-hold check ends;
  pixels alone let the fixture press a key early and abort its own capture.
  Pointer motion does not change pressed state; the echo proves commit and
  actual input dispatch before key/button assertions, without a new product
  telemetry callback on the semantic writer.
- Physical XKB key decoding formatted candidate names repeatedly. Direct
  four-byte row decoding preserves the previous physical key mappings, including
  special/keypad precedence. On TCG, the benchmark changed from 518,119 ns/op,
  872 B/op and 87 allocations/op to 1,828 ns/op and zero allocations. This is a
  microbenchmark, not an end-to-end handoff latency claim.
- A valid peer disconnect closed the transport before retiring its active
  lease, allowing a concurrent heartbeat to write to the closed socket. The
  lease now retires under the writer coordinator before deferred transport
  closure. A deterministic test injects a heartbeat at closure: it fails on
  the old implementation and passes after the correction.
- The first full rerun passed interoperability but exposed another teardown
  race in lifecycle cases. A heartbeat write failure closed the raw TCP socket;
  subsequent source/session owners closed it again and received `net.ErrClosed`.
  `PeerConnection` now shares one `sync.Once` close operation, including the
  failed-write paths, and retains its original error for every caller. Eight
  concurrent close callers must issue exactly one native close; an actual
  first-close error remains an error, rather than being normalized away.
- A queued input could race liveness retiring its lease, producing `not inbound
  active`. The dispatcher now checks cancellation before dispatch and reports
  completed cleanup separately from the original dispatch cause, matching its
  existing read-error recovery contract. Confirmation requires pending work to
  join and recovery to return only the exact supplied cause. A release failure
  invalidates that confirmation. Original dispatch failures remain available
  independently from completed cleanup evidence.
- Receiver-lock runs then exposed an interrupted XTEST request closing its
  connection. Key/button cleanup reused that closed owner and failed; the
  independent crashguard eventually released the holds, so checking pixels or
  held-state alone would miss the failure. Linux release now uses one separate
  recovery connection for idempotent key/button UP only, on closed/EOF failures.
  Press, movement and wheel are never retried or rearmed. Both connections close
  with the input owner; a canceled cleanup, a closed owner or failed recovery
  remains an error. A real Xvfb test closes the first owner while two tokens are
  held, verifies actual release, checks presses/motion remain blocked, and checks
  that a failed recovery cannot claim success. It passed on TCG in 0.26 seconds.
- After native cleanup succeeded, an interrupted input's original EOF still
  made an authoritative lifecycle shutdown look like `session_failed`. A typed
  parent-cancellation proof now retains original errors for diagnostics while
  recording which cause won first. It requires LOCAL, an empty pressed ledger,
  joined workers, released native owners and no exceeded recovery deadline.
  Only validated lifecycle transitions may consume this proof. Unexpected
  protocol/liveness/injection failures and additional cleanup errors invalidate
  it. No test exit expectation or native release assertion was relaxed. The
  receiver-lock regression passed in 26.87 seconds before the final full run.
- A later run isolated a genuine X11 query failure just after reconnection.
  Closing the failed source client released its grabs, but the agent treated
  that verified retirement as unverified and abandoned its connection request.
  The X11 connection and Desktop now retain the real first close result; only
  `net.ErrClosed` from an already closed socket is normal retirement. Failed
  operations, cancellation and Desktop shutdown share that close owner. The
  Linux hook reports completed retirement only after successful close. The
  original query failure remains visible and the supervisor can reconnect with
  its existing bounded backoff. Windows hooks do not generate this proof.
  Native tests close an acquired/grabbed owner without canceling its parent,
  retain the original error and prove a new X client can reacquire the grabs.
  Concurrent-close and injected-close-failure tests reject false evidence.
- A later focused receiver-lock run still failed after reconnect (33.69 s);
  it is not counted as passing evidence. Cancellation during X11 startup,
  before source-hook readiness, could close the native owner but omit its
  retirement proof. Startup now uses the same single close owner, joins its
  cancellation callback before publishing the XGB connection, and retains the
  original close error. Source-hook cleanup covers pre-readiness failures too;
  overlapping Run calls cannot inherit another owner's retirement proof.
  The focused receiver-lock run then passed in 30.05 seconds. Unit tests reject
  failed startup-close evidence and overlapping setup without opening X11.

### Final TCG execution evidence

The release-profile (`CGO_ENABLED=0`) agent/CLI and compiled test executables
ran on the unchanged one-vCPU, 1-GiB guest. The final prepared execution used
the same private 1280x720 and 1024x768 X servers, two real UIDs, real PAM/logind
sessions, and the unchanged 8-minute test timeout. No scenario exit expectation,
held-token assertion, input-grab assertion or readiness deadline was relaxed.
The six actual lifecycle scenarios all passed in **163.89 seconds**:

| Actual logind action | Source | Receiver |
| --- | ---: | ---: |
| `lock-session` | 28.22 s | 25.23 s |
| `SetLockedHint` | 26.83 s | 26.85 s |
| `terminate-session` | 29.04 s | 27.72 s |

Actual CLI interoperability also passed in that execution in **18.55 seconds**. Its assertions
covered mutual pairing, authenticated transport, native entry/movement,
keyboard and button down/up, four wheel directions, return and held-input
disconnect cleanup. The separate native release/retirement tests and the
Linux execution of cancellation/close evidence tests passed before those
scenarios. Full output is retained on the disposable VM as
`/home/validator/tcg-final-final.log`; the earlier 162.20-second pass preceded
the final cancellation-proof hardening, and previous failed runs are not counted
as passes. A preceding full script invocation independently demonstrated one
agent/CLI prebuild (247 seconds on TCG) before the timed scenarios, replacing
the seven repeated pair builds; its later failed case is retained separately.

Windows focused package tests, Linux cross-vet and test compilation, Go
formatting, `git diff --check`, and workflow `actionlint` passed. HTTP/CLI
schemas, DB storage and CI job count are unchanged. The X11 test script changes
only build placement, fixture lifetime and admission/polling ownership.
Linux VM `CGO_ENABLED=1 go test -race ./internal/x11desktop
./internal/transport -count=1 -timeout 2m` also passed (11.491 s and 17.245 s
package execution; compilation separate). These race checks cover deterministic
close/transport tests; the opt-in native scenarios are covered by the separate
Xvfb execution above. Windows race execution was unavailable because the host
has CGO disabled and no C compiler; it is not reported as a pass.

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
