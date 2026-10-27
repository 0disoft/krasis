# ADR 0072: Independent heartbeat admission for an active lease

- Status: Accepted; physical-device verification pending
- Date: 2026-10-28
- Extends: ADR 0005, ADR 0020 and ADR 0071

## Context

An input operation retained both the runtime owner and semantic writer permit
while waiting for its native ACK. Heartbeat dispatch and sending waited behind
that operation. A bounded read-ahead channel could also fill before dispatch
resumed, stopping the only socket reader. Native input delay could consequently
be classified as a silent peer, whose cleanup then closed the pending ACK.
The g471 evidence does not establish why the first positive ACK read was late.

## Decision

The authenticated reader may renew only the exact, active, unexpired session's
immutable lease proof using CAS. It cannot grant authority, acknowledge input,
advance input sequences or release pressed state. Pending, foreign, malformed
and retired heartbeats retain ordered dispatch, including ADR 0020's drain.

The runtime continues to own input admission, injection, ordering and joined
recovery. Local input ACK waits no longer retain the semantic writer permit.
Heartbeat sends inspect the immutable proof under that permit; semantic control
writes keep their existing ordering. Sending alone never renews receive liveness.
Older successful input receipts cannot overwrite more recent heartbeat progress.

Expiry must seal the exact proof before stopping its session. A heartbeat cannot
revive a sealed proof, and a stale watchdog snapshot cannot expire a renewed one.
The native input deadline remains two seconds regardless of continuing heartbeats.

The reader uses at most 64 queue entries. Each entry combines at most 64 adjacent
pointer sequences of the same session and epoch within the input delta bounds.
Keys, buttons, wheels, heartbeats, control frames and invalid sequences are
barriers. Zero-sum runs preserve sequence coverage and net displacement using two
nonzero moves. No key or control frame is dropped. A full queue cancels joined
input after 100ms with `reconnect_input_queue_timeout`, rather than silently
blocking heartbeat reads until peer expiry. This can disconnect a severely
overloaded peer and requires physical testing.

## Verification and migration

Deterministic regressions keep six 250ms heartbeat send/receive cycles running
while input is blocked for 1.5 seconds. Other checks cover independent input
timeout, joined cleanup, expired and foreign identities, expiry/renewal fencing,
queue barriers and 2,000 oscillating moves with every sequence preserved.

Peer wire framing and authentication do not change. No database, installer,
public API, dependency or runner change is required. Physical latency and
recovery-frequency improvements remain unverified; race detection is unavailable
with the current CGO-disabled toolchain.
