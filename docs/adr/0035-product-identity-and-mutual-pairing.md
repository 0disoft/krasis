# ADR 0035: 제품 기기 신원과 양방향 확인 페어링

- 상태: Accepted
- 결정일: 2026-09-01
- 범위: 제품 기기 신원, pairing ceremony, trust lifecycle
- 관련 ADR: `0006-persistent-trust-store.md`, `0032-live-trust-authorization-and-certificate-validity.md`

## 배경

Phase 0의 역할별 인증서는 24시간 실험 자산이고, 한 장비가 bundle을 만들어 상대 장비로 복사하는
절차다. 이 절차는 설치별 장기 신원, 양쪽 사용자 확인, 교체 장비와 key rotation을 소유하지 않으므로
제품 pairing으로 재사용할 수 없다.

## 결정

- 현재 사용자 범위의 제품 data root는 `%LOCALAPPDATA%\Krasis`다. `identity`, `trust`,
  `diagnostics`를 별도 data class directory로 둔다.
- 제품 root, identity directory, identity document와 lock은 현재 사용자와 `SYSTEM`만 허용하는
  protected DACL을 사용한다. 기존 root의 ACL이 느슨하거나 path가 reparse-backed이면 자동 수정하지
  않고 실패한다.
- 설치별 device identity는 Ed25519 key pair와 단조 증가 `generation`이다. private key는
  `identity\identity.json` 안에서만 보관하고 UI, CLI, report, pairing payload로 내보내지 않는다.
  일반 상태 API는 schema, SHA-256 public-key fingerprint, generation과 생성 시각만 반환한다.
- device identity에는 자동 만료를 두지 않는다. 의심되는 노출, 장비 양도·분실, ACL 실패,
  암호 정책 변경 또는 명시적인 사용자 rotation에서 새 key와 다음 generation을 원자 교체한다.
- transport 인증서는 device identity와 분리한다. pairing offer와 response는 각 device public key와
  현재 transport certificate fingerprint·역할을 하나의 5분 transcript에 묶는다.
- 양쪽 화면은 transcript에서 계산한 같은 6자리 confirmation code를 보여준다. 양쪽 사용자가 정확한
  code를 확인해야 각 device identity가 transcript receipt에 서명한다.
- local confirmation과 검증된 remote receipt가 모두 있어야 remote transport fingerprint를 trust
  store에 import한다. 한쪽 확인, 다른 pairing receipt, 만료, 변조, public-key/fingerprint 불일치는
  fail-closed한다.
- alias와 inbound-control capability 수정은 한 trust revision에서 수행한다. 교체 장비는 기존
  fingerprint 폐기와 새 fingerprint 등록을 같은 revision에서 수행하며 기존 fingerprint를 부활시키지
  않는다.
- Phase 0 credential/trust directory 자동 migration은 지원하지 않는다. 실험 identity를 제품 identity로
  승격하면 단기 key의 출처와 복사 범위를 증명할 수 없기 때문이다. 실행을 중지하고 양쪽 Phase 0 trust를
  폐기한 뒤 각 장비에서 제품 root를 새로 만들고 다시 pairing한다.

## 복구

- identity JSON 손상, unknown field, trailing JSON, key mismatch, oversized document, permissive ACL,
  reparse path와 concurrent mutation은 빈 identity나 이전 key로 fallback하지 않는다.
- rotation 뒤 상대 peer trust는 자동 변경하지 않는다. 기존 active lease를 관리 API로 종료하고,
  상대 장비에서 기존 fingerprint와 새 fingerprint를 atomic replacement한 뒤 새 session을 시작한다.
- 장비 분실은 남은 모든 peer에서 기존 transport fingerprint를 revoke한 뒤 replacement pairing을 한다.

## 결과

장기 device identity와 짧은 transport credential의 수명을 분리하면서도 pairing 승인 대상을 exact
transport fingerprint로 고정한다. 대가로 rotation과 transport renewal은 상대 장비 확인이 필요한
명시적인 작업이며, UI와 제품 CLI는 ADR 0036의 local agent API를 통해 이 core를 호출해야 한다.

## 검증

- protected default root 생성, existing insecure root와 reparse-backed directory 거부
- identity create/load/atomic rotation, corruption·unknown·trailing·oversized·key mismatch 거부
- identity lock contention 거부
- 동일 confirmation code, local·remote 양쪽 receipt 필수, 만료·변조 거부
- alias/capability update와 revoke+replacement의 단일 trust revision
