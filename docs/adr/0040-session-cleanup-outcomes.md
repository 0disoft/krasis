# ADR 0040: Preserve all session worker outcomes

- Status: Accepted
- Scope: Session shutdown error propagation

Cancellation selects a first cause, not the complete cleanup outcome. RunSession
records each started worker's return independently, joins workers, and preserves
non-cancellation failures alongside the first cause. A worker returning nil after
cancellation is not reclassified as an unexpected stop. Pure expected cancellation
trees are omitted; joined cleanup failures and independent cleanup deadlines are
not suppressed merely because errors.Is also finds context.Canceled.

The supervisor preserves the session error when the parent was canceled. A
non-LOCAL final state also preserves its underlying causes. This change does not
claim that a LOCAL runtime snapshot proves native cleanup; product recovery
classification and the management reconnect latch are a subsequent change.

Regression witnesses: a hook returns joined cancellation/cleanup failure after
local cancellation or an authenticated peer disconnect; a supervisor session
returns cleanup failure after parent cancellation. All three lost the cleanup
error before the change. Tests mock native input and do not install hooks.
