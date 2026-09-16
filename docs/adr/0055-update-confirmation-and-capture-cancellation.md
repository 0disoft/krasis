# ADR 0055: Confirm replacement runtime before reconnecting

Generation 28 reconnects the input session only after the authenticated management
channel confirms the exact target generation and revision (`UPDATED`), and only
while the local input state is LOCAL. DELIVERED and TRANSFER_UNKNOWN do not grant
permission to reconnect. A late runtime confirmation can resolve a previously
unconfirmed restart. A FAULTED local session still requires verified recovery.

The existing pinned `/v2/status` exchange adds optional `updateID` and
`updateState` fields. Only APPLY_FAILED is advertised, only to the transfer's
paired peer. The sender accepts it only for the matching transfer ID and peer,
while waiting for runtime confirmation, and shows PEER_APPLY_FAILED. Old peers
omit the fields; their failures remain unconfirmed rather than invented success.
No arbitrary error text, file paths, or other peers' transfer IDs are exchanged.

Capture shutdown now wraps a terminal cause instead of joining it with an
independent descriptive error. Joining a plain message with context cancellation
made the existing cancellation-only classifier treat ordinary shutdown as native
failure. Real mixed cleanup errors remain failures; worker joining, native cleanup
and the FAULTED latch are unchanged. Deterministic tests reproduce that distinction.

The reported repeated foreground-unavailable activation still requires native
two-PC verification. These changes do not prove that every capture failure is
fixed or justify bypassing foreground ownership checks.
