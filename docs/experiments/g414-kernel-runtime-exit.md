# g414: wait for the exact authenticated runtime's kernel exit (#16)

A controlled child served the real local API protocol, acknowledged upgrade-stop,
closed its listener and singleton, but retained a native image-file pin while
waiting for an explicit test exit. Existing Client.StopForUpgrade returned success
while the parent still could not rename that file. This reproduces insufficient
exit evidence, not attribution of g410/g413's actual blocking handle.

On Windows the client now pins a SYNCHRONIZE-only process handle before STOP.
The fixed IPv4 loopback listener's kernel PID is resolved, a fresh non-pooled,
non-proxied, non-redirected Runtime request authenticates the same instance nonce,
and the listener owner/alive handle are rechecked. A successful accepted stop
waits on that exact handle under the caller context. API disappearance and
singleton release alone are no longer completion evidence for this request.
Cancellation returns failure, not exit proof. No process is killed or selected
from a caller-provided PID; no PID/address/handle is added to the API or logs.

The native table parser follows Microsoft's documented
[GetExtendedTcpTable](https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getextendedtcptable)
and [MIB_TCPROW_OWNER_PID](https://learn.microsoft.com/en-us/windows/win32/api/tcpmib/ns-tcpmib-mib_tcprow_owner_pid)
layouts. Retrieval has three attempts and an 8 MiB maximum. Malformed counts,
ambiguous/zero owners, wildcard/other ports and self-process selection fail
closed. Non-Windows behavior is unchanged. This is a local client lifecycle
change; peer/control wire schemas and publisher trust are unchanged.

Verification: the old implementation failed with success before kernel exit.
Real child tests now cover actual exit, cancellation while the image is still
held, and changed-instance rejection. Native table boundary tests pass, as do
localapi/productpanel/appinstall/residentagent suites and localapi/productpanel/
appinstall vet. These are native handle/file lifetime tests with simulated API
and input ownership; no physical key/button was injected.

## Boundaries and next unit

Startup.Available still uses named singleton availability when no API owner can
be authenticated at entry. That separate no-API path needs read-only resident/
image-owner quiescence evidence before installation. It was not started after
the ten-minute cutoff. Do not infer full #16 completion, replace an unavailable
owner with an arbitrary PID, or weaken image pins. Other handle owners and the
remaining signing/device lifecycle requirements stay open. #50/#59 and the
physical/UI/endpoint matrix remain unresolved. The Windows Security handoff and
attended matrix availability questions are still pending.

No API/DB schema, dependency, runner, firewall, signing, repository hygiene,
branch/worktree, remote ref, push or issue comment changes. Source/build and
paired-application evidence are recorded separately below. Race detection is
unavailable with CGO disabled; hosted CI is not run because push is forbidden.
