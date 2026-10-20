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
