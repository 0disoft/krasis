# Krasis data integrity

- Status: Active
- Owner: repository maintainers

## Persistent data

Current persistent product data consists of local device credentials, peer trust records, bounded launcher configuration, and local diagnostic reports. Krasis has no product database and no cloud-owned state.

Runtime session identity, lease ownership, epochs, input sequence, liveness, pressed-input journals, cursor-entry snapshots, and cleanup state are ephemeral and must never be restored as an active lease after restart or reconnect.

## Write contracts

Credential and trust-store writers validate the complete model before persistence, use restrictive local permissions, serialize concurrent writers, write a new file, flush and close it, then replace the destination atomically for the supported platform.

Report and evidence writers use bounded schemas, exclusive creation, destination validation, and post-write verification. A report may not be silently appended, partially accepted, overwritten, or promoted after validation fails.

Probe-kit artifacts are addressed by the exact clean commit. An existing artifact path is reusable only when its verified contents match the staged payload byte-for-byte; mismatched evidence fails closed.

## Recovery contracts

A failed write leaves the last accepted persistent state intact or returns an explicit error that prevents the caller from claiming success. Temporary files and locks are recovery details, not authoritative state.

Trust revocation takes effect for new authorization checks immediately. Reconnect never restores an old lease, sequence, liveness deadline, or pressed-input journal.

## Privacy and provenance

Persistent reports contain only allow-listed aggregate fields. They must not contain raw input, private credentials, local usernames, absolute paths, addresses, device identifiers, window data, or copied pass declarations.

Hardware evidence records the clean revision and verified artifact identity so results cannot be attributed to different code.

## Known gap

Path validation currently rejects unsafe existing components, symlinks, and Windows reparse points, but validation followed by path-based replacement still leaves a same-user replacement race. Issue #17 owns the handle-relative or equivalent Windows write primitive and adversarial regression tests. Until it is accepted, same-user malicious processes are outside the integrity guarantee.
