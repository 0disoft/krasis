# CLI Tool

- Status: Draft
- Repository Type: cli-tool

## Repository Type Contract

This repository type owns command behavior, arguments, flags, config loading, exit codes, terminal output, JSON output, runtime compatibility, and shell integration contracts.

## Source of Truth

- Product decision: product pairing is resident-agent mediated; command surface remains issue #14
- Technical owner: UNASSIGNED
- Related ADR: `docs/adr/0035-product-identity-and-mutual-pairing.md`

## Required Decisions

- Command list and flag ownership: UNDECIDED
- Exit-code taxonomy: UNDECIDED
- Machine-readable output contract: UNDECIDED
- Config precedence and default behavior: UNDECIDED
- Runtime compatibility floor: UNDECIDED

Issue #14 will expose product pairing through the resident agent API. That CLI may show device
fingerprints, aliases, capabilities, pairing expiry and the human confirmation code, but must never
print private keys or certificate bodies. The existing `krasis-lab peer-credentials` and `trust`
commands remain Phase 0 tools and are not the product migration path.

## Review Blockers

- A command changes without updating help, examples, output, and exit-code expectations.
- JSON output exposes generated or existing file contents.
- Runtime compatibility changes without smoke validation.
