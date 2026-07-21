# Security Baseline

- Status: Draft

## Contract

Security baseline covers authentication, authorization, tenant boundaries, input validation, output validation, secrets, external integrations, logs, and security blockers.

## Required Evidence

- Phase 0 source of truth: `docs/product/02-spec.md`, `docs/security/desktop-security.md`,
  `docs/engineering/08-threat-model.md`, `docs/adr/0003-ordered-peer-transport.md`,
  `docs/adr/0004-phase-0-crash-guard.md`, `docs/adr/0006-persistent-trust-store.md`,
  `docs/adr/0032-live-trust-authorization-and-certificate-validity.md`
- Owner: UNASSIGNED
- Merge-blocking validation: VALIDATION.md
- Related checklist: CHECKLIST.md

## Enforced Phase 0 Baseline

- peer transport는 TLS 1.3 mTLS와 `krasis/1` ALPN을 모두 통과해야 하며 system root fallback을
  사용하지 않는다.
- inbound control은 인증서 fingerprint, 역할, 명시적 capability와 폐기 상태를 protected trust
  store에서 검사한다. reload 오류는 이전 allowlist로 되돌아가지 않고 fail-closed한다.
- 역할별 private key와 trust store는 현재 Windows 사용자와 `SYSTEM` 전용 protected DACL을
  사용하고 load 전에 reparse path와 file type을 검증한다.
- crash guard IPC와 report에는 normalized pressed token 또는 aggregate counter만 허용하며 원시 key,
  입력 문자열, pointer 좌표·trajectory, clipboard와 화면 내용은 금지한다.
- report output은 기존 file을 덮어쓰지 않고 부모 path의 symlink·Windows reparse point를 거부한다.
- 인증서 만료는 새 handshake와 credential load를 막지만 disk의 private key를 자동 삭제하지 않는다.
  Phase 0 local-data 삭제 책임은 `docs/desktop/local-data.md`를 따른다.

## Deferred Product Decisions

정식 pairing 승인 UI, installer ACL, code signing, update trust, firewall rule, elevation과 active lease의
관리자 강제 폐기는 아직 `UNDECIDED`다. 이 문서의 Phase 0 baseline을 제품 지원 계약으로 승격하지 않는다.

## Review Blockers

- A change bypasses the source of truth.
- A change weakens validation or hides skipped checks.
- A change lacks failure, recovery, security, performance, or test evidence where relevant.
- A change persists raw input, credentials, private keys, certificate fingerprints or local paths in aggregate reports.
- A change treats certificate expiry as proof that credential files were erased.
