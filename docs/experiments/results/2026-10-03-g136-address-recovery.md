# g136 recovery observations: HP15 and GRAM13

- Date: 2026-10-03, Asia/Seoul.
- Product generation: 136 on both PCs.
- Product revision: `e3c27ec31591b6b9dedaf4e70800a1cd561ddd9b`.
- Evidence: live CLI responses, fresh sanitized support exports from both PCs,
  and the operator's display/input confirmation. This document is a bounded
  observation, not a complete #70 validation or a release claim.

## UI recovery

The operator quit both management apps through the tray while keeping the
agents running. A's local UI process count was zero; its CLI reported
`ui.state:NOT_RUNNING`, a `VERIFIED` launch record and `CONNECTED` input.

A was relaunched through its fixed per-user CLI. B was opened through
`app open B`. Both commands returned `launch_requested`, then fresh support
exports showed new `launch_record_ready` events with new run identifiers on
both PCs. A's doctor reported `healthy`, a running g136 UI and a matching g136
agent. The operator confirmed both windows appeared and bidirectional mouse
and keyboard input worked. This does not test recovery after agent termination.

## DHCP renewal without an address change

The observed topology is A/listener and B/connector: A's trusted B record has
role `CONNECTOR`, and A's saved endpoint belongs to its own active DHCP IPv4
interface. Physical input ownership does not determine this TLS role.

At 18:20:39 KST, one adapter-scoped `ipconfig /renew` completed with native
exit code 0. The IPv4 address remained unchanged. A's sampled connection state
was `CONNECTED` before and after renewal and again at the later collection.
No IP release, static address assignment or adapter reset was performed.

Fresh exports at 18:22:41 KST (A) and 18:22:42 KST (B) contain no post-renewal
address-search/rebind, reconnect or session-stop event for this product build.
The current and previous agent logs were complete. Both exports marked the
older `control.previous.jsonl` incomplete, so no claim is made about that
older UI history. Screen movement after this renewal was not independently
observed; this is a state/event observation over about two minutes.

An A-side UDP probe found an endpoint matching A's saved address. Because
A is the listener, that is consistent with its own advertisement; it is not
evidence that B received the advertisement or authenticated a changed endpoint.

## Remaining #70 acceptance conditions

- Change A's actual listener address on the same private IPv4 LAN and NIC,
  using an address legitimately available on that network and a recovery plan.
- Without manually editing B's saved address or clicking Connect, observe
  A's listener rebind and B's discovery/authenticated address update.
- Confirm bidirectional input, unchanged trusted identity, layout and rearm
  settings, and safe cleanup of held inputs.
- Wrong-identity candidate refusal remains covered by local regression tests;
  this observation did not introduce another identity on the physical LAN.

#70 remains open. DHCP renewal with an unchanged address does not satisfy the
actual endpoint-change condition. Current source contracts:
[LAN address recovery](../../desktop/address-recovery.md) and
[Microsoft ipconfig reference](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/ipconfig).
