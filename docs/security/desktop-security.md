# Desktop Security

- Status: Draft
- Repository Type: desktop-app

## Repository Type Contract

This repository type owns installed app behavior, OS support, local data, installer, auto-update, crash reporting, permissions, and desktop-specific security contracts.

## Source of Truth

- Product decision: UNDECIDED
- Technical owner: UNASSIGNED
- Related ADR: `docs/adr/0004-phase-0-crash-guard.md` (Phase 0 범위)

## Required Decisions

- Supported operating systems and architectures: UNDECIDED
- Installer and update channel ownership: UNDECIDED
- Local data and cache ownership: UNDECIDED
- Crash report and diagnostic data policy: UNDECIDED
- Desktop permission and security boundaries: Phase 0 crash guard는 agent와 같은 로그인
  사용자 권한, 상속된 익명 pipe와 동일 executable만 사용한다. 정식 설치 ACL, code signing,
  single-instance와 elevation 경계는 UNDECIDED다.

## Review Blockers

- Installer, update, or OS support behavior changes without platform-specific validation.
- Local data behavior changes without migration, privacy, and recovery notes.
- Crash diagnostics or logs can expose private data.
