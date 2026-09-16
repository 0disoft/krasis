# Paired management sharing

Status: accepted

## Diagnostic delivery

An explicit local diagnostic export also queues the sanitized support snapshot
for the selected paired device. It does not grant update or clipboard permission.
The existing pinned mutual-TLS management channel remains independent of input
state. Every request rechecks current paired trust. No remote source path or
destination filename is accepted.

Local authenticated POST `/v1/diagnostic-transfer` accepts only a peer fingerprint.
The resident agent exports its own logs. The management route of the same name
uses POST to upload, GET to pull, and PUT to acknowledge durable receipt by job ID.
There is one outgoing snapshot, capped at 1 MiB, expiring after one minute.
Retries retain the same ID; duplicate receipt is idempotent within the process.
Old peers return unsupported; network loss retains local export and expires the job.

The receiver reprojects the support schema, source names, fixed event codes and
validated boundary fields. Unrecognized data never reaches the saved document.
One file per paired certificate is atomically replaced in the diagnostics folder:
`support-peer-<fingerprint>.json`. No received content is executed. Success means
receiver storage acknowledgment, not merely bytes written to a connection.

Tests cover trust rejection, peer binding, duplicate delivery and sanitization.
Two-PC delivery and native UI presentation require manual verification.
