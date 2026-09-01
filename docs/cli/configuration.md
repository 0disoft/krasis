# CLI configuration

- Status: Active for Phase 0 lab tooling
- Owner: repository maintainers
- Command contract: `docs/cli/command-contract.md`
- Output contract: `docs/cli/output-and-exit-codes.md`

## Current boundary

`krasis-lab` is a bounded diagnostic and peer-session tool. It is not the final product CLI and must not silently become a long-running production agent. The production CLI, stable configuration file, precedence rules, and authenticated local-agent management path are tracked by issues #13 and #14.

Current defaults used by the executable belong to the packages that consume them, principally `internal/agent`, `internal/labcli`, `internal/peer`, and `internal/wininput`. Documentation must name those defaults but must not create a second independent value source.

## Input sources

Lab commands accept explicit flags and command arguments defined in `docs/cli/command-contract.md`. Credentials, private keys, trust stores, and report destinations must use validated local files rather than embedding secret material in command lines, examples, environment variables, or logs.

Unknown or malformed flags, invalid ranges, conflicting roles, unsafe output paths, and missing required identity material fail before a listener or live input session starts.

## Output and privacy

Human output may explain the failing stage and safe recovery action. Machine output must follow the stable schemas and exit codes in `docs/cli/output-and-exit-codes.md`.

Neither output mode may emit raw keystrokes, text, pointer trajectories, certificate private material, local usernames, absolute paths, peer addresses, window handles, thread identifiers, or device identifiers. Reports may contain only the aggregate counters and bounded classifications owned by their validator packages.

## Unresolved production decisions

| Decision | Owner | Blocker | Decision trigger |
| --- | --- | --- | --- |
| Production config file location and format | Phase 5 agent/CLI owner | Resident agent and local API are not implemented | Issue #13 API contract is accepted |
| Flag, environment, file, and built-in precedence | Phase 5 CLI owner | Stable production command surface is not implemented | Issue #14 command review |
| Product default listener and peer discovery behavior | Pairing and network owners | Pairing UX and public-LAN hardening are unresolved | Issues #10 and #11 are accepted |
| Stable JSON versioning policy for product commands | CLI owner | Current schemas are lab-specific | First production CLI release candidate |

Until those triggers, new lab-only options must remain explicit, bounded, documented, tested, and incompatible with accidental production use.
