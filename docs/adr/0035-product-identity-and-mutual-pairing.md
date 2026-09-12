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

### 제품 공개 인증서 저장 구현

`Session.FinalizeWithCertificate`는 기존 양쪽 확인 검증 후, transport adapter가 받은 공개 leaf DER의
지문·역할이 transcript와 정확히 일치하는지 확인한다. DER는 offer/response/receipt에 추가하지 않는다.
제품 인증서는 self-signed end-entity signing certificate로 제한하고 CA 인증서는 거부한다.

공개 DER는 protected trust directory의 `certificate-FINGERPRINT.der`에 저장한다. trust-store lock을
잡은 채 인증서 저장을 먼저 완료하고 마지막에 trust.json을 원자 교체한다. 인증서 저장 실패는 trust를
등록하지 않는다. 반대로 마지막 trust write가 실패하면 권한이 없는 공개 DER만 남을 수 있으며,
재시도는 동일한 protected bytes만 재사용한다. 손상된 파일을 자동 덮어쓰지는 않는다.

`Store.PeerCertificate`는 매 호출 시 현재 trust의 fingerprint·role·revocation과 인증서 만료를
재검증한다. 인증서 파일이 존재한다는 사실만으로 trust를 부여하지 않는다. 반환한 인증서는 해당 peer의
명시적 TLS trust anchor로 사용할 수 있다. 전역 CA 설치나 TLS 검증 생략은 하지 않는다.

기존 fingerprint-only Finalize와 trust.json 형식은 유지한다. 제품 network pairing adapter는
FinalizeWithCertificate 경로를 사용하며 제품 session runner와 연결되어 있다.
기존 fingerprint-only peer의 인증서를 자동 가져오거나 갱신하지 않는다.

### 인증서를 포함한 교체 코어

`Session.FinalizeReplacementWithCertificate`는 새 peer의 양쪽 확인과 공개 DER를 검증하고,
`Store.ReplaceWithCertificate`는 같은 lock 안에서 인증서를 준비한 뒤 기존 지문 철회와 새 지문
등록을 단일 trust revision으로 기록한다. 기존 peer가 없거나 철회되었거나 새 지문이 이미 등록되어
있으면 거부한다. 재시도로 철회된 신뢰를 부활시키지 않는다.

인증서 검증·준비 실패 및 최종 trust 교체 전 중단은 이전 trust와 연결 설정을 변경하지 않는다.
인증서 저장 직후 중단으로 남은 DER는 권한이 없으며 동일한 protected bytes로만 재시도할 수 있다.
최종 원자 교체 이후의 검증 오류는 이미 commit되었을 수 있으므로, 호출자는 실패를 무조건 rollback으로
해석하지 않고 현재 trust를 다시 조회해야 한다. 두 장비의 저장을 하나의 transaction으로 보장하지 않는다.

이 코어는 호출자가 선택한 기존 지문을 교체한다. 기존 peer 선택의 사용자 승인과 active session 종료는
호출자의 책임이며, 새 peer 확인 receipt 자체가 기존 지문 선택을 승인하는 것은 아니다.
기존 연결 profile은 삭제하거나 새 지문으로 자동 이전하지 않는다. 교체 성공 뒤 새 peer는 별도의 연결
설정이 필요하다. 인증서 갱신, 교체용 CLI/API와 부분 실패 복구 흐름은 후속 작업이다.

- protected default root 생성, existing insecure root와 reparse-backed directory 거부
- identity create/load/atomic rotation, corruption·unknown·trailing·oversized·key mismatch 거부
- identity lock contention 거부
- 동일 confirmation code, local·remote 양쪽 receipt 필수, 만료·변조 거부
- alias/capability update와 revoke+replacement의 단일 trust revision
