# ADR 0067: Register fixed UI recovery independently of the renderer

The previous window-ready callback registered an empty revision and discarded
registration errors. A missing recovery directory or unverifiable record left
the fixed relaunch command unavailable even when the UI was running.

g135 registers from primary UI service startup using the actual Go build
revision. Registration remains separate from agent startup and renderer
readiness. A registration failure does not terminate a working input session.
Diagnostics contain a fixed failure stage only; paths and raw errors are omitted.
The logger supports nil receivers when logging itself is unavailable.

Prepare the fixed CLI and command script before publishing current.json. A
failed registration keeps the previous target record. This is a publication
ordering guarantee, not an atomic transaction across all three files.

The explicit offline `app repair` command verifies its adjacent update bundle
and matches its generation/revision to the executing CLI before registration.
It accepts no path or control-directory override and sends no remote command.
It deliberately selects that bundle, so operators must use the latest verified
package. No service, input lease, connection or security policy is changed.

Local regression coverage includes startup without a renderer or logger,
missing CLI, preservation of the previous record, invalid bundles and CLI
arguments, build mismatch and successful fixed-entrypoint creation. Live
two-device recovery remains a separate deployment check.

## g136: Inspect live directories without delete access

A g135 running UI recorded `launch_record_image_failed`. A read-only probe of
that image reproduced Win32 error 32 at the parent-directory open, before
opening the EXE. The generic secure directory opener requests DELETE and
WRITE_DAC, although image verification only needs to read. Existing handles
that omit delete sharing therefore prevent this inspection. We have not
identified which process owns the conflicting handle.

Use a separate least-access directory opener for the launch image, source CLI
and record reads. The same handle-relative file operations, reparse refusal,
hard-link checks and SHA-256 verification remain. Mutation paths retain their
existing access requirements; sharing flags, ACLs and security policy are not
changed. Deterministic Windows tests hold a directory without delete sharing,
reproduce the original failure and verify registration, inspection and relaunch
validation with the new read access.

- [Windows file sharing and access rights](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
