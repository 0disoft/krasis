# g383: resume the stopped peer through the existing verified update handoff

The g382 rollback restored the g380 implementation, but A's explicit disconnect
before replacement also stopped B's input session. A is CONNECTING while B's
management channel remains online. B had already recorded protected source
readiness before peer_disconnect_requested/session_stopped; no new g382 fault
was observed. This is distinct from the rejected g381 startup failure.

The existing remote app-open request only shows the remembered window. Its
single-instance gate intentionally does not prepare or reconnect an existing
window. Update receipt comparison rejects same-generation reapplication.

Use the already-authorized development update handoff as recovery generation
383, with identical production code to g380 apart from AgentGeneration. Its
existing updated-workspace path verifies the exact build and current trusted
peer fingerprint before resuming the connection, preserving local manual-stop
and uncertain-result fences. No new remote command, caller-selected endpoint,
trust grant or network permission is introduced. Do not send Disconnect on A
after B resumes. A's verified safe-stop/replacement lets B retain its retry
intent rather than interpreting an explicit peer disconnect.

This generation change exists to recover the stopped peer through an available
authenticated mechanism; it is not a product release/version promotion. Package
validation and actual paired connection/source readiness remain required. No
push, issue comment, branch or worktree creation.
