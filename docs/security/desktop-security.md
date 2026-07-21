# Desktop Security

- Status: Draft
- Repository Type: desktop-app

## Repository Type Contract

This repository type owns installed app behavior, OS support, local data, installer, auto-update, crash reporting, permissions, and desktop-specific security contracts.

## Source of Truth

- Product decision: UNDECIDED
- Technical owner: UNASSIGNED
- Related ADR: `docs/adr/0004-phase-0-crash-guard.md`,
  `docs/adr/0006-persistent-trust-store.md`,
  `docs/adr/0007-windows-agent-lifecycle-owner.md`,
  `docs/adr/0032-live-trust-authorization-and-certificate-validity.md` (Phase 0 범위)

## Required Decisions

- Supported operating systems and architectures: UNDECIDED
- Installer and update channel ownership: UNDECIDED
- Local data and cache ownership: 정식 제품 default directory는 `UNDECIDED`다. Phase 0 launcher,
  report, credential과 repository cache의 보존·삭제 경계는 `docs/desktop/local-data.md`를 따른다.
- Crash report and diagnostic data policy: 정식 제품 정책은 `UNDECIDED`다. Phase 0은 local
  aggregate-only report, no automatic upload와 수동 evidence retention만 허용한다.
- Desktop permission and security boundaries: Phase 0 crash guard는 agent와 같은 로그인
  사용자 권한, 상속된 익명 pipe와 동일 executable만 사용한다. Phase 0 역할별 credential
  directory와 private key는 Windows에서 현재 사용자와 `SYSTEM` 전용 protected DACL을
  사용하고 load 전에 DACL과 reparse path를 검증한다. 역할 directory 아래 trust store의 부모,
  directory, lock과 document에도 같은 protected DACL과 reparse path 검증을 적용한다. report output도
  부모 path의 reparse point를 거부한다. trust 폐기는 다음 inbound lease authorization부터 적용하고
  credential은 유효기간 밖에서 load하지 않는다. 정식 설치
  ACL, code signing, single-instance와 elevation 경계는 UNDECIDED다. Phase 0 lifecycle monitor는
  별도 권한 상승 없이 현재 interactive session의 숨겨진 window로 power, WTS session, shutdown과
  display message를 받고 첫 안전 관련 event에서 현재 session을 폐기한다.
- Phase 0 network exposure: listener의 unauthenticated handshake는 동시 4개와 연결당 5초로 제한하지만
  IP별 rate limit은 구현하지 않는다. 신뢰하는 사설 LAN 또는 개인 hotspot에서만 실행하고, 공용 LAN은
  Windows 방화벽으로 connector 주소만 허용할 수 있을 때만 사용한다. launcher는 방화벽을 변경하지 않으며
  자동 규칙 생성, 공용 Wi-Fi DoS 방어와 주소별 limiter는 정식 제품 운영 결정으로 남긴다.

## Review Blockers

- Installer, update, or OS support behavior changes without platform-specific validation.
- Local data behavior changes without migration, privacy, and recovery notes.
- Crash diagnostics or logs can expose private data.
