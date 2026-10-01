# ADR 0066: Product leases use the installed input service

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
