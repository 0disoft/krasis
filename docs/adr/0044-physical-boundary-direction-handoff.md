# ADR 0044: Reverse direction at a physical boundary crossing

Status: Accepted

## Behavior

With A controlling B, B's own mouse/touchpad may cross its configured edge to
start controlling A without disconnecting. The same behavior applies in reverse.
Small local movements do not immediately steal control; the existing physical
boundary detector and admission gate remain the trigger. Tagged injected input
is still excluded by the existing source input pipeline.

The boundary beginner now accepts an idle INBOUND_ACTIVE lease as well as LOCAL.
After the normal local menu/capture/pressed-input admission check, it revokes the
exact inbound lease, writes REVOKE, then begins a fresh outbound REQUEST on the
same ordered connection. Source capture activates only after GRANT, as before.
The idle return cursor shield is not shown during this direction handoff.

Remote pressed keys/buttons block handoff; the runtime checks again at revoke to
cover a press racing admission. Release held inputs and re-enter the edge rather
than forcibly ending a drag. Pending, faulted and already-outbound states cannot
start another lease. Failed revoke writes do not start a new request. Existing
retired-stream draining is extended to outbound pending/active states for the
distinct old identity. Old input still validates epoch and sequence and is never
injected; old heartbeats cannot refresh the new lease's liveness.

## Validation

Tests verify LEFT/RIGHT handoff, exact old identity, REVOKE-before-REQUEST order,
no capture before grant, no idle cursor shield, held input and failed write.
Existing boundary admission, input filtering and lease regression tests remain.
Two-PC mouse/touchpad handoff, repeated reversals, scrolling, drag rejection and
disconnect need manual validation. Protocol schema is unchanged. Generation 16.
