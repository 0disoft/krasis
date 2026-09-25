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

## Clipboard

Clipboard sharing is independently opt-in, persisted for one selected paired
certificate on each device. Update permission never grants clipboard access.
The GUI explicitly warns about secrets; applications marking clipboard content
`ExcludeClipboardContentFromMonitorProcessing` are excluded. Unicode text is
limited to 64 KiB UTF-8 and rejects NUL or invalid UTF-8. PNG and ordinary
uncompressed DIB images are bounded to 40 MiB and 10 million pixels. Explorer
file and folder lists are transferred as a bounded 40 MiB ZIP of actual file
bytes, with at most 512 entries and no reparse points, symlinks or special files.
File contents are unpacked into a private local clipboard directory on the
receiving PC and removed after 24 hours while Krasis runs, or at the next
start after that deadline. Text and image clipboard content is
never persisted or logged by Krasis; file content is never logged.

The authenticated local `/v1/clipboard` API provides ENABLE, DISABLE and SYNC.
Both agents advertise permission and latest change metadata in management status
before transporting content on `/v1/clipboard`. Random change IDs bind requests,
receipts and duplicate suppression. One outgoing and one incoming value are kept
in memory with 15-second and 10-second deadlines. There is no offline replay.
Concurrent local copies win over pending remote copies; simultaneous conflicting
copies may require copying again rather than silently choosing a device winner.
Image and file metadata is advertised only to peers that support each format;
older agents continue to exchange supported formats. Upgrading from a version
whose consent did not include files resets clipboard sharing until the user
enables it again on both PCs.

Native clipboard sequence is checked while the clipboard is open before a remote
write, preventing an in-flight response from replacing a newer local copy. A
remote-format marker and the exact post-write sequence prevent echo. The GUI
acknowledges an incoming ID after SetClipboardData succeeds. Network receipt only
means the bounded agent inbox accepted the content, not that an app pasted it.
Enabling/restarting baselines the current clipboard without transmitting it.

Only the unlocked Default input desktop is used. No clipboard read or write is
attempted on lock/secure desktop. The GUI heartbeat permits transport for five
seconds; an already-authorized request has a twelve-second deadline. Content copied
before locking can therefore finish an already-admitted transfer, but locked
  desktop contents are never sampled. The GUI must remain running or in tray.

Unit tests cover opt-in, cross-peer denial, revocation, expiry, duplicate IDs,
local-copy conflicts and malformed content. Native clipboard ownership, lock and
unlock, and two-PC copy/paste remain manual checks.
