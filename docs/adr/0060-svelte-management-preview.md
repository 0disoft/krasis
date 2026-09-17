# ADR 0060: Isolated Svelte management UI

- Status: Accepted for preview only

Implement the user-selected dark device-arrangement design in frontend/ using
Svelte 5 and Vite, with Korean/English catalogs. Preserve the existing Go/Win32
management binary and resident agent until native bridge and lifecycle parity
are verified. This supersedes ADR 0001's no-rewrite preference only for the
isolated management UI experiment, not input processing or production cutover.

The preview is clearly marked, has no local API credentials, and performs no
real connections, diagnostic exports, clipboard reads, permission grants or
updates. Language is the only locally persisted preview setting. Status and
versions must be supplied by the agent in the integration stage; do not claim
latest-version status or generate real pairing PINs from the renderer.

Use system font fallback and full-sentence catalog keys; keep physical screen
placement independent of writing direction. No cloud service or telemetry.
Existing product icon is reused. Phosphor provides standard UI icons.

Rollback is deletion of the independent frontend directory. The Windows ZIP,
Go dependencies, local API and generation remain unchanged for this stage.
Build and catalog tests do not substitute for visual browser QA or two-PC tests.
