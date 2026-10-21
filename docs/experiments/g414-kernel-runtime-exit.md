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

## Bundle and paired application

The completed g414 bundle passes offline update verification at source revision
c1b2a83354a6e97fe077b99c9a7a440efafa362d, 28,320,630 bytes. Existing focused
verification above was retained; unchanged source tests were not rerun.

A's single automatic takeover attempt began at 02:40:20Z on 2026-10-21.
Its installation receipt at 02:40:31.0413547Z reports complete, result 0,
recovery not_needed. No manual UI close, safe-stop, process termination or
installation retry was needed. The installed agent SHA-256 matches the bundle.
Fresh doctor reports healthy CONNECTED, g414 UI/resident/installed builds all
matching, verified launch record, running service and completed stage cleanup.
This successful upgrade does not identify the earlier blocking handle or prove
that all folder replacement failures have been eliminated.

B update 31aa9fed9731002b1481d3bef16e8d36 reports UPDATED at the same revision.
Its installation receipt at 02:41:03.2285485Z reports complete, result 0,
recovery not_needed. Fresh peer status independently reports ONLINE at g414.
Collection f2f449d97dfcbdd7c7742c1c2a8aa9e0 at 02:41:18Z/19Z contains 24
current-build events per PC, the same session correlation, READY input health
on both and no current-build fault/startup-failed event. These are bounded event
observations, not a physical-input or long-duration soak result.

## Issue disposition and handoff

Remote inspection still finds no open PRs; local and remote-tracking branch
inventory contains only main. No remote refs were changed. Dashboard #1 remains
open as requested. #4/#5/#6/#7/#15/#16/#20/#50/#59/#70 remain open because their
acceptance evidence is incomplete; no comments or unsupported closures were made.
The user's successful A/B traversal, click, keyboard, drag and wheel report is
g377 evidence only and is not relabeled as g414 acceptance.

Next priorities remain (1) authenticated no-API resident/image-owner quiescence
for #16, (2) the remaining native callback/desktop lifetime audit for #50/#59,
and (3) attended lifecycle and real endpoint-change trials for #6/#70. Signing
and exact-revision hosted CI remain pending; pushing is still forbidden. The
Windows Security and attended-matrix handoffs remain unanswered. This unit ends
with both PCs restored to the connected current build; no new source unit was
started during the deployment verification.
