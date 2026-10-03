# ADR 0066: Product leases use the installed input service

## g138: Show portable fallback after updates

The status API and desktop snapshot expose the runner's selected input backend.
Portable selection shows a persistent PIN/administrator-window setup notice
and links to installation instructions in the device update dialog. Installing
the matching protected images requires attended Windows elevation after safe
shutdown. Updating the app alone does not perform that installation.

The observation is fenced to the current session and cleared at completion.
Missing or unfamiliar values remain unknown, including older agents. It is
neither permission to inject nor proof of worker lease readiness or PIN control.
Image matching, installed-agent selection and input authorization are unchanged.

## g137: Distinguish portable protection causes

The portable session guard records one fixed cause code per runner, from the
same observation that detected protection:

- `protected_input_desktop_unavailable`: the input desktop could not be opened
  or its name could not be queried.
- `protected_input_non_default_desktop`: the queried desktop is not Default.
- `protected_input_credential_broker`: the foreground credential broker was
  positively identified.
- `protected_input_identity_unknown`: foreground process identity could not
  be established while its window was still foreground, or its PID was unknown.

These codes survive sanitized support export; no process path, window title,
credential or input content is recorded. The first cause remains stable even
if monitor and input checks race or the foreground changes before cancellation.
Later checks still inspect current state rather than caching a protection
decision. Unknown identity still stops the portable session. The installed
worker path and recovery policy are unchanged.

Older `protected_input_detected` records alone cannot distinguish these causes.
New real-device evidence is needed before attributing a disconnect to a
credential dialog or treating unknown identity as a false positive.

The g104 local Esc probe succeeded with the required desktop access mask. That
did not enable product A-to-B input: portable agents still injected at Medium
integrity and stopped sessions when a credential prompt was observed.

g107 connects the product path. After an attended elevated installer has placed
the fixed agent and input service, the UI prefers that agent only when its
SHA-256 matches the verified sibling bundle. Protected images remain pinned
through process creation. Missing, invalid or older installations retain the
portable path and its conservative protected-input guard.

Only the installed ordinary console agent can select privileged injection.
Its actual process path, token session and administrator-owned image digest
are checked. The SYSTEM service independently authenticates the kernel pipe
client and the exact installed image. Service readiness alone is not permission
to inject: the resident agent first approves the authenticated mTLS peer's
inbound lease, then creates an unpredictable binding for that exact identity.

The immutable delegate sends press, release, motion and wheel events through
the authenticated worker. Local cursor entry and return hysteresis stay in the
existing Windows adapter, including held-button suppression. A lost reply is
never replayed or sent through ordinary SendInput. Worker-owned crash recovery
replaces the ordinary crash guard's duplicate ledger for delegated presses.

While this installed path is selected, a credential prompt no longer cancels
the product connection. A worker heartbeat runs every 250 ms, bound to the
approved session context. Failure cancels the connection. Worker lifetime is
limited by certificate expiry and 55 minutes, below the one-hour protocol cap.
Matching return, lease abort or session cleanup closes the worker; stale returns
cannot close a current lease. Revocation remains enforced by resident trust
checks and context cancellation. No input payload, PIN, window text or clipboard
content is logged.

Automatic update still transports the installer without executing it. Installing
or replacing protected binaries requires the receiving user's Windows elevation
consent. Starting g107 alone does not install the service. After installation,
stop the portable agent safely while disconnected and let g107 start the matching
installed agent. A new revision requires matching installed binaries; an older
service is never silently trusted for a newer portable agent.

Focused tests use fake injection and authenticated in-memory channels; they do
not enter credentials, elevate, install a service or demonstrate remote PIN
success. Real two-PC mouse, keyboard, Esc, return and release-recovery checks
remain required, especially cursor operations while a security dialog is open.

## g108: Ordinary-agent service authentication

The receiving service still checks the ordinary agent's actual token. In the
opposite direction, the ordinary agent no longer reads the SYSTEM service's
token or requests process synchronization access. B's earlier Deskflow token
comparison reported access denied from ordinary privilege; product bootstrap
must not assume that cross-account token query is available.

The service client reads the named pipe's kernel server PID and requires that
PID to be the running fixed own-process LocalSystem service. It verifies SCM
configuration and its administrator-owned DACL, the fixed quoted image command,
session 0, actual process image path, expected image digest and protected file
ownership. A retained limited-query process handle checks GetExitCodeProcess,
without PID lookup on subsequent requests. The pinned service itself checks
SYSTEM/System integrity and restricted/AppContainer state before readiness.
No service/process/token ACL or Windows security policy is changed.

References: [process access rights](https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights)
and [service access rights](https://learn.microsoft.com/en-us/windows/win32/services/service-security-and-access-rights).
