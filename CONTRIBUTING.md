# Contributing to Krasis

- Status: Active
- Owner: repository maintainers

## Scope

Krasis is a Windows peer-input application. Changes can affect whether a user keeps local keyboard and pointer control, so safety-state transitions, input release, authentication, and diagnostic privacy are review-critical.

The product contract is `docs/product/02-spec.md`. Accepted decisions live in `docs/adr/`. Runtime ownership and failure flow live in `docs/architecture/02-runtime-flow.md`. Code and deterministic tests are executable evidence; generated files, caches, build output, and unverified hardware reports are not source truth.

## Before editing

1. Read `AGENTS.md`, `VALIDATION.md`, `CHECKLIST.md`, and `.agents/context-map.md`.
2. Read the product, architecture, ADR, security, performance, and testing documents touched by the change.
3. Confirm the issue can be completed without inventing hardware results, signing infrastructure, credentials, local paths, IP addresses, or device identifiers.
4. Keep one independently reviewable contract or behavior per pull request. Stack a PR only when its base dependency is explicit.

## Change requirements

Every pull request must state what changed, why, source-of-truth documents, API and persistent-data impact, repository-hygiene or runner impact, risk, validation, skipped validation with reasons, and rollback.

Changes to session state, lease ownership, input ordering, release behavior, trust authorization, listener exposure, local data, or Windows lifecycle must update the matching tests and accepted ADR or explain why the existing decision still applies.

Physical-device gates in `docs/experiments/` require evidence from the named devices. Unit tests, simulated input, CI runners, and copied historical reports cannot replace those gates.

## Validation

Use the commands in `DEVELOPMENT.md`. The pull-request workflow is authoritative for Windows formatting, tests, vet, both probe binaries, probe-kit packaging, verification, and artifact upload.

Documentation-only changes still run the repository workflow because examples and contracts name real packages, commands, and artifacts.

## Privacy and evidence

Do not commit private keys, certificates, raw keystrokes, input text, pointer trajectories, local usernames, absolute paths, IP addresses, hostnames, device identifiers, or unredacted reports. Commit only aggregate evidence allowed by the relevant report schema and experiment contract.
