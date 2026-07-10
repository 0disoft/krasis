# Desktop App

- Status: Draft
- Repository Type: desktop-app

## Repository Type Contract

This repository type owns installed app behavior, OS support, local data, installer, auto-update, crash reporting, permissions, and desktop-specific security contracts.

## Source of Truth

- Product decision: UNDECIDED
- Technical owner: UNASSIGNED
- Related ADR: UNDECIDED

## Required Decisions

- Supported operating systems and architectures: UNDECIDED
- Installer and update channel ownership: UNDECIDED
- Local data and cache ownership: UNDECIDED
- Crash report and diagnostic data policy: UNDECIDED
- Desktop permission and security boundaries: UNDECIDED

## Review Blockers

- Installer, update, or OS support behavior changes without platform-specific validation.
- Local data behavior changes without migration, privacy, and recovery notes.
- Crash diagnostics or logs can expose private data.
