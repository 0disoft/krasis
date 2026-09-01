# Dependency and change policy

- Status: Active
- Owner: repository maintainers

## Current baseline

The Go module currently uses the standard library only. Win32 integration is performed through narrow syscall adapters owned by this repository. GitHub Actions are external build dependencies and are pinned to immutable commit SHAs with their reviewed release version recorded in comments.

## Adding a dependency

A new runtime dependency requires a concrete capability the standard library or existing adapter cannot provide, a maintained upstream, acceptable license, vulnerability review, bounded transitive graph, Windows support, and a removal plan.

Input-path, authentication, credential, installer, update, serialization, or native-code dependencies require an explicit security and failure-boundary review. A convenience wrapper is not sufficient justification for moving ownership out of `internal/control` or `internal/application`.

## Version policy

Use the supported major release range as the intentional compatibility boundary. Stable updates within the accepted major should be adopted after CI validation. A new major is promoted deliberately after its behavior, permissions, runner compatibility, and migration impact pass the repository workflow.

Workflow actions remain pinned to full immutable SHAs even when the documented compatibility boundary is a major release. Renovation must update the SHA and version comment together.

## Change discipline

Keep dependency-only changes isolated from product behavior. Record runtime, binary-size, startup, latency, privacy, and rollback impact where relevant. Do not regenerate unrelated files or commit module, build, or cache output.

Removing a dependency must also remove dead configuration, permissions, scripts, documentation, and exception paths.
