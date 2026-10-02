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
