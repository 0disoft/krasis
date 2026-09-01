# Krasis testing standard

- Status: Active
- Owner: repository maintainers
- Validation commands: `DEVELOPMENT.md`
- Workflow: `.github/workflows/ci.yml`

## Evidence layers

Deterministic unit tests own pure transition, sequence, liveness, pressed-input, serialization, validation, and file-model behavior.

Integration tests own framed peer sessions, authenticated connection behavior, write ordering, reconnect identity, supervisor cleanup, and subprocess failure handling.

Windows tests own Win32 structure mapping, hook and message routing, injection result classification, lifecycle translation, path permissions, launcher behavior, and Windows-only builds.

Physical-device experiments own actual capture, suppression, touchpad wheel behavior, cursor transfer, suspend and lock recovery, network interruption, crash recovery, latency, compatibility, and soak claims. CI and simulated input cannot satisfy these gates.

## Merge-blocking minimum

Every change runs formatting, `go test ./...`, repository vet, both Windows builds, probe-kit packaging and verification in the pull-request workflow.

A behavior change requires a focused test at the lowest layer that can prove it. A bug fix requires a regression test unless the only valid proof is a named physical gate. A contract or documentation change must keep commands, package names, schemas, and acceptance criteria synchronized with executable code.

Changes to safety state, authentication, authorization, input delivery, cleanup, persistent trust, path handling, or report privacy require negative-path tests. Do not add broad skips, sleeps as synchronization, network-dependent tests, or a repository-wide coverage target.

## Physical evidence rules

Hardware reports must identify the exact clean revision and verified probe-kit artifact without committing private machine data. Aggregate validators must reject missing stages, inconsistent totals, unexpected fields, mixed revisions, and copied or hand-edited pass claims.

An implementation agent may improve the kit, validator, or report template, but must not mark a physical issue complete without results collected on the required devices.

## Skipped validation

A pull request must name every skipped validation, the reason it could not run, and the remaining risk. “Not applicable” must point to the untouched boundary. A skipped Windows or hardware gate cannot be rewritten as a pass.
