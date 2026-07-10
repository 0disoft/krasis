# Repository Design Scaffold

- Status: Draft
- Scope: general
- Repository Type: desktop-app
- Addons: cli-tool

This repository contains an LLM-friendly design scaffold. It is not application source code.

## Source Files

- AGENTS.md: agent working rules
- CHECKLIST.md: checklist router
- VALIDATION.md: validation names and reporting requirements
- .agents/context-map.md: agent route map
- docs/: design, operations, architecture, and engineering standards

## Repository Shape Notes

- desktop-app: This repository type owns installed app behavior, OS support, local data, installer, auto-update, crash reporting, permissions, and desktop-specific security contracts.
- cli-tool: This repository type owns command behavior, arguments, flags, config loading, exit codes, terminal output, JSON output, runtime compatibility, and shell integration contracts.


## Repository Hygiene

.editorconfig, .gitattributes, and .gitignore are generated to keep line endings,
binary diffs, local files, build outputs, caches, and secret files under control.

## Scope Notes

Project-specific implementation choices remain UNDECIDED until the repository owner records them.
