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
allocation/handle ownership, malformed results, live read-only sampling,
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
