# ADR 0032: Live Trust Authorization and Certificate Validity

- 상태: Accepted
- 결정일: 2026-07-20
- 범위: Phase 0 peer credential load, bounded session lifetime와 inbound lease authorization
- 대체: ADR 0006의 process 시작 시 immutable grant snapshot 결정

## 배경

ADR 0006은 `agent-session` 시작 때 trust store를 한 번 읽고 같은 authorizer를 모든 재연결과 lease에
재사용했다. 별도 `trust revoke` process가 store revision을 올려도 이미 실행 중인 agent는 다음 process
시작 전까지 폐기를 보지 못했다. TLS handshake는 인증서 유효기간을 검사하지만 credential load와
`trust import`는 handshake 전의 만료·미개시 인증서를 명시적으로 거부하지 않았다.

## 결정

- `agent-session`은 시작 시 trust store를 strict하게 읽어 초기 정책 오류를 조기에 실패시킨다.
- 각 inbound `LeaseRequest` authorization은 같은 protected trust store를 다시 strict하게 읽고,
  fingerprint, 역할, `allowInboundControl`과 `revokedAt`을 그 revision에서 평가한다.
- reload의 lock contention, DACL·reparse 검증, decode, schema 또는 정책 조립 오류는 allowlist를
  재사용하지 않고 해당 lease authorization을 fail-closed한다.
- reload는 lease control path에만 있다. keyboard, pointer, button, wheel과 heartbeat 고속 경로에는
  file I/O를 넣지 않는다.
- 이미 승인되어 active인 lease를 store file watcher로 비동기 종료하지 않는다. 폐기는 다음 inbound
  lease authorization부터 적용하며 active lease 강제 종료 UX는 정식 제품 pairing·관리 결정으로 남긴다.
- identity load와 `trust import`의 peer certificate load는 현재 시각이 certificate `NotBefore`보다
  이르거나 `NotAfter`보다 늦으면 명시적으로 실패한다. TLS handshake의 표준 chain·hostname·기간
  검증은 그대로 유지한다.
- `agent-session`은 identity가 요청한 bounded 실행 종료 시각까지 유효하지 않으면 listener, dialer와
  live input session을 시작하지 않는다. crash guard와 input adapter 준비 사이에도 다시 검사하고 같은
  종료 시각을 run context deadline으로 사용해 준비 지연 때문에 만료 뒤 재연결을 허용하지 않는다.
- Phase 0은 인증서 수명을 자동 연장하거나 실행 중 갱신하지 않는다. 남은 수명이 부족하면 새 Pair를
  생성하는 것이 복구 절차다.
- trust store v1에는 certificate 만료 시각을 복제하지 않는다. 만료된 certificate는 TLS와 load에서
  사용할 수 없고 새 certificate는 다른 fingerprint를 가지므로 기존 record가 새 identity를 승인하지 않는다.

## 결과

실행 중 폐기된 fingerprint가 같은 agent process의 후속 lease에서 다시 승인되지 않는다. 대가로 inbound
lease마다 작은 보호 file을 읽고 process lock을 획득하며, 동시에 관리 mutation이 진행 중이면 안전하게
lease를 거부하거나 session을 종료할 수 있다. 즉시 active lease를 끊는 관리 채널은 아직 없다.
요청한 실행보다 남은 인증서 수명이 짧으면 `credential_lifetime_insufficient`로 조기에 종료하므로,
실험 중 만료에 따른 불명확한 reconnect 실패 대신 새 Pair 생성이 필요하다는 원인을 보존한다.

## 검증

- 같은 authorizer instance에서 active record 승인 뒤 document 폐기를 반영한 다음 lease 거부
- trust store reload 실패 때 기존 allowlist를 재사용하지 않는 fail-closed 동작
- identity와 trusted peer certificate의 유효기간 전·후 load 거부
- identity `NotAfter`와 정확히 같은 종료 시각 허용, 그 이후 종료 시각 거부
- 유효기간 안의 기존 credential load와 mTLS 검증 회귀
