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
  `.trust.lock`을 소유한다. Phase 0 launcher는 executable 옆 `launcher-config.json`과 사용자가 지정한
  상대 경로의 aggregate report만 소유한다. 정식 제품 default directory와 다른 local data는 UNDECIDED다.
- Crash report and diagnostic data policy: 정식 제품 정책은 `UNDECIDED`다. Phase 0 report는 local
  aggregate JSON만 허용하고 자동 upload하지 않으며 아래 보존·삭제 경계를 따른다.
- Desktop permission and security boundaries: trust store directory와 file은 현재 Windows 사용자와
  `SYSTEM` 전용 protected DACL을 사용한다. 정식 설치 경계는 UNDECIDED다.

## Trust Store Recovery

손상, unknown schema, DACL 불일치와 lock contention은 허용 목록을 비운 성공 상태로 바꾸지 않고
명시적으로 실패한다. `trust revoke`는 record를 삭제하지 않는다. backup, export, restore와 schema
migration UX는 제품 trust store 경로를 확정할 때 별도 결정한다.

## Phase 0 Launcher Data

`launcher-config.json`에는 역할, 장비 별칭, peer IP, port, edge, 실행 시간과 executable 기준 상대
credential/report directory만 저장한다. 인증서 본문, private key, trust fingerprint와 report 내용은
config에 복제하지 않는다. 저장은 같은 directory의 임시 file을 flush한 뒤 Windows replace로 교체한다.

launcher는 자신이 시작한 현재 실행의 agent/guard report 절대 경로를 memory에서만 추적한다. 보고서 복사는
그 두 JSON object를 `krasis.launcher.report-bundle/v1`의 `agent`, `guard` field로 묶으며 local path를 넣지
않는다. 재시작 뒤 오래된 report를 자동으로 골라 복사하지 않는다.

## Phase 0 Retention and Deletion

| 경로 | 내용 | 기본 보존과 삭제 경계 |
| --- | --- | --- |
| `launcher-config.json` | 역할, alias, LAN IP, port, edge, duration, 상대 directory | 다음 설정 저장까지 유지한다. private key나 report 본문은 없지만 local network·경로 정보가 있으므로 공유하지 않는다. |
| `reports/` | aggregate agent, guard와 진단 JSON | 실패 분석과 사용자 전달이 끝날 때까지 보존한다. 자동 삭제하지 않으며 필요한 report를 복사한 뒤 사용자가 directory를 삭제할 수 있다. |
| `secrets/pair-*`와 복사한 역할 directory | CA, identity certificate, private key, peer certificate와 trust store | certificate 유효기간이 끝나도 file은 남는다. 실험과 필요한 재현이 끝나면 양쪽 노트북에서 pair directory 전체를 삭제한다. Git, report, cloud-sync와 공유 directory에 두지 않는다. |
| repository `.cache/go-build/` | 구성된 검증이 쓰는 Go build cache | 제품 데이터나 evidence가 아니다. 다른 `mf run`이 실행 중이지 않을 때 언제든 재생성 가능하게 삭제할 수 있다. |
| repository `.cache/*-smoke/`, `.cache/github-artifacts/` | local smoke의 aggregate report와 내려받은 build artifact | source of truth가 아니다. 실패 조사나 artifact 비교가 끝나면 삭제할 수 있다. credential과 private key를 넣어서는 안 된다. |
| `build/`, `dist/` | 재생성 가능한 executable과 package | 배포 source가 아니다. 실행 중인 process와 필요한 artifact 전달이 끝난 뒤 삭제할 수 있다. |

launcher는 증거 보존과 다른 노트북에서 사용 중인 pair 여부를 알 수 없으므로 이 경로들을 시작 또는 종료 때
일괄 삭제하지 않는다. 정식 제품의 보존 기간, secure deletion과 cleanup UI는 설치·data directory를
확정할 때 별도 결정한다.

## Review Blockers

- Installer, update, or OS support behavior changes without platform-specific validation.
- Local data behavior changes without migration, privacy, and recovery notes.
- Crash diagnostics or logs can expose private data.
- Certificate expiry or trust revoke is treated as on-disk credential deletion.
