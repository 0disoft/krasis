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

## Paired recovery verified

Both PCs applied g383, revision 3f6dc1e9c5a8de6331fc4dd0b255605b86729c05.
Package verification accepted 27,968,478 bytes and B receipt
b1110beefa0accc9c19cc0a495b8d6f5 reports UPDATED. A installer returned 0 and
its installed agent hash matches. No Disconnect was sent during A replacement.
Doctor reports healthy/CONNECTED with matching UI, agent and installed build.
Paired collection at 20:47:18Z/19Z reports READY input health and completed
cleanup on both sides. A has one protected-source-ready event; B has two (the
rolling replacement required reacquisition). Neither current-build collection
contains fault_/session_failed. Input connection recovery is complete; this is
not a new physical-input or long-soak acceptance claim. All ten issues remain
open and g381's rejected changes remain out of the current production source.
