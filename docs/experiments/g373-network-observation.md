# g373: sampled local power/path observations and one serial retry wake

The product connection owner now samples system power source, battery-saver
state and local non-loopback interface availability every two seconds. It logs
initial values and subsequent changes as fixed codes, independently of
transport faults and heartbeat expiry. Failed inspection is unknown, never an
observed outage. Interface enumeration order does not create a false change.
Local interface/address changes are compared with a process-local digest; no
address, interface name, MAC, digest, SSID/BSSID or OS error text is persisted.

System power uses GetSystemPowerStatus; it does not expose a wireless adapter's
device power state. That state is explicitly `network_adapter_power_unknown`.
Roaming and firewall policy evidence are independently unknown as well. An AC
transition, battery saver or generic timeout is never declared the cause of a
network failure. Polling can miss short transitions and is not an OS event
trace. `network_link_usable` means a local up interface with a unicast address,
not successful connectivity to the authenticated peer.

Observed offline-to-usable or changed local paths can wake one existing
supervisor retry wait. Notifications are coalesced to capacity one, expire
after two seconds and never start a dial or cancel an active session. Existing
LOCAL/cleanup checks still precede the next serial acquisition, which creates
new session/lease ownership. A state notification cannot revive an old lease,
validate a heartbeat or turn an uncertain cleanup into success. Power changes
alone do not wake a retry. Monitor shutdown cancels and joins its sampler.

Tests cover restoration, one-shot consumption, stale/unknown observations,
cancellation, fixed privacy vocabulary, path comparison, sampler shutdown and
native power ABI/unknown results. Netobserve, productlog, residentagent and
agent suites pass. No injected event is claimed as actual WiFi sleep/roaming.
This addresses #67's bounded observation/unknown attribution and recovery
notification requirements. #68/#71/#72 still require their more specific
roaming, selected-route and policy observation providers; generic local path
changes do not complete them.

No public API, DB, dependency, firewall/power setting or CI runner changes.
The existing support/v1 event vocabulary gains fixed labels only. Hosted CI,
physical power/session fault matrices and signed release gates are unverified.
No issue comments or Git push; paired installation evidence follows packaging.

Native references:
- https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getsystempowerstatus
- https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-system_power_status

## Paired installation and issue disposition

Both PCs run g373 at revision
`184593abf40d7866e07a7d0ec0ef0dd2363eafa3`. Offline verification accepted
the 27,900,571-byte bundle and B transfer
`3e63bf2b56685fa7bd4e3c609ad99be7` reports UPDATED at that revision.
Native packaging, installer provenance, UI tests and the eight-file update
package check passed. A's installer returned 0; the installed agent SHA-256
matches the bundle, and CLI/UI/running/installed agent/recovery record match.

Both collected reports contain `network_power_ac`, `network_saver_off` and
`network_link_usable`, plus explicit unknown adapter-power/roaming/policy
events. The observations are genuine local API results on each PC; no WiFi
power-saving episode, AP roam or firewall block was claimed or induced.
Collections at 18:47:46Z and 18:48:32Z/30Z on 2026-10-20 show input health
and service READY on both PCs, with no g373 fault/session failure. A doctor
is healthy/CONNECTED and B is ONLINE. The short window does not prove
long-term stability or the remaining hardware matrix.

Issue #67 is closed without a comment for its explicit unknown attribution,
separate OS/transport observations and safe restoration notification handling.
Thirteen issues remain open. The next network priority is #68: a real WLAN
association/roaming provider that compares AP identity only in memory and
publishes fixed observation codes, reusing this one-shot serial retry wake.
Selected-route and policy attribution remain #71/#72. No additional task was
started beyond this execution's ten-minute cutoff; the working tree is clean
after this record. No Git push or issue comments.
