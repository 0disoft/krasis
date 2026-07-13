# Local Data

- Status: Draft
- Repository Type: desktop-app

## Repository Type Contract

This repository type owns installed app behavior, OS support, local data, installer, auto-update, crash reporting, permissions, and desktop-specific security contracts.

## Source of Truth

- Product decision: `docs/adr/0006-persistent-trust-store.md` for peer trust records
- Technical owner: UNASSIGNED
- Related ADR: `docs/adr/0006-persistent-trust-store.md`

## Required Decisions

- Supported operating systems and architectures: UNDECIDED
- Installer and update channel ownership: UNDECIDED
- Local data and cache ownership: Phase 0 trust store는 명시적인 directory의 `trust.json`과
  `.trust.lock`을 소유한다. 정식 제품 default directory와 다른 local data는 UNDECIDED다.
- Crash report and diagnostic data policy: UNDECIDED
- Desktop permission and security boundaries: trust store directory와 file은 현재 Windows 사용자와
  `SYSTEM` 전용 protected DACL을 사용한다. 정식 설치 경계는 UNDECIDED다.

## Trust Store Recovery

손상, unknown schema, DACL 불일치와 lock contention은 허용 목록을 비운 성공 상태로 바꾸지 않고
명시적으로 실패한다. `trust revoke`는 record를 삭제하지 않는다. backup, export, restore와 schema
migration UX는 제품 trust store 경로를 확정할 때 별도 결정한다.

## Review Blockers

- Installer, update, or OS support behavior changes without platform-specific validation.
- Local data behavior changes without migration, privacy, and recovery notes.
- Crash diagnostics or logs can expose private data.
