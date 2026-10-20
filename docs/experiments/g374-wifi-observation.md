# g374: permission-aware WLAN association observations

The existing two-second network sampler now reads Windows WLAN interface
state. It queries current association attributes only when the non-prompting
AppCapability.CheckAccess result for wiFiControl is already Allowed. It never
requests location access, scans, changes radio/profile/settings, or registers
MSM notifications. API/service/permission failures stay unobserved; known
absence, disconnected, connecting and connected states remain distinct.

On the same interface, adjacent readable samples distinguish a changed AP on
the same SSID from changing networks. A permission/observation gap breaks this
comparison; reconnecting on another AP after that gap is not labelled an
observed roam. A sampled AP change does not prove an 802.11 roaming handshake,
its exact time, or that it caused transport loss. Short changes between samples
can be missed. Missing observations never mean no roaming occurred.

Typed diagnostics include per-monitor HMAC interface/AP pseudonyms, fixed
change reason, sample-window duration and observation-gap duration. Raw GUID,
SSID/BSSID, profile, hashes and OS error text are not persisted. Entropy failure
produces explicitly unknown identifiers. Export validates the vocabulary,
identifier shape and durations; extra native identity fields are discarded.
These times describe observation, not measured packet-loss/roam duration.
Existing transport progress/retry events can be compared by their timestamps.

AP/network changes and restored association share the existing capacity-one,
two-second-expiry supervisor wake. No event dials, cancels an active session,
validates a heartbeat or resumes an old lease. Native observation has a
one-second wait limit and at most one process-wide outstanding query. A stuck
native query may finish later; its private result cannot reach another owner,
and no replacement query/thread is spawned while it is outstanding. Session
teardown joins the observation loop without waiting forever for native RPC.

Tests cover native ABI, permission rejection without an association query,
handle ownership, malformed results, live read-only sampling,
same-SSID AP change vs network change, blind gaps, single retry wake,
per-monitor anonymity and export rejection of raw/invalid fields. Synthetic
tests are not evidence of an actual roam or WiFi-caused packet loss. The live
A query reported connected with association observation available.

No public control API, DB, dependency, firewall/power setting or CI runner
changes. Support/v1 gains a validated WiFi summary. No issue comments or push.
Physical roaming/fault matrices, signed release and hosted CI remain unverified.
Paired installation and issue disposition follow package verification.

References:
- https://learn.microsoft.com/en-us/windows/win32/nativewifi/wi-fi-access-location-changes
- https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapabilityaccessstatus
- https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_connection_attributes

## Paired installation and acceptance

Both PCs run generation 374 at revision
`a8dc062ddafdd7ef657001f57fb8a497de608a0a`. Offline verification accepted
the 27,891,928-byte bundle; B transfer
`6468872e0041f1d7d44e8eda34685586` reports UPDATED at that exact revision.
UI tests, native packaging, installer provenance and the eight-file update
bundle check passed. A's installer returned 0, the installed agent SHA-256
matches the bundle, and CLI/UI/running/installed/recovery versions match.

Both actual reports show `network_wifi_connected`, `network_roaming_available`
and a valid connected WiFi summary with 16-character anonymous interface/AP
identifiers. The IDs themselves are not copied into this document. No permission
request, scan, forced disconnect or actual AP roam was induced. The successful
native samples are evidence that observation works on both PCs, not evidence
that either PC roamed or that WiFi caused a particular failure.

Collections at 19:05:37Z/36Z and 19:06:21Z/20Z on 2026-10-20 show input
health and service READY on both PCs with no g374 fault/session failure. A
doctor is healthy/CONNECTED and B is ONLINE. The short window does not establish
long-term stability or satisfy the physical fault/soak matrix.

Issue #68 is closed without a comment for conditional association observation,
explicit unobserved states, privacy-bounded identity/change/gap summaries and
one-shot serial restoration retry behavior. Twelve issues remain open. Next
network priorities are selected-interface/address-family attribution (#71)
and evidence-based socket/policy classification (#72). Native injection (#50),
the remaining hook callback audit (#59), endpoint hardware acceptance (#70),
GUI/release gates (#15/#16) and the physical matrix (#4/#5/#6/#7/#20) remain.
No new task was started after the ten-minute cutoff. No API, DB, dependency,
runner, firewall or power-setting changes; no Git push or issue comments.
