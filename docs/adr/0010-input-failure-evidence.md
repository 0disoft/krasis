# ADR 0010: 입력 실패와 cursor rollback을 집계 증거로 남긴다

- 상태: Accepted
- 결정일: 2026-07-14
- 범위: Phase 0 `agent-session` report와 strict checker

## 맥락

기존 report는 성공한 destination 입력 수만 기록했다. `SendInput` 실패가 session을 종료해도 실패
확실성이 report에 남지 않았고, grant 실패 뒤 cursor rollback이 실제 복원됐는지, 최신 물리 이동을
보호하려고 복원하지 않았는지, rollback 자체가 실패했는지도 checker가 구분하지 못했다. 성공 수만
보면 안전 cleanup 실패를 정상 실험으로 오판할 수 있다.

## 결정

- `krasis.lab.agent-session/v7`은 destination 입력 실패를 `NOT_APPLIED`, `PARTIALLY_APPLIED`,
  `OUTCOME_UNKNOWN`, 미분류의 누적 개수로만 기록한다.
- cursor rollback은 시도, 복원, 미복원, 실패의 누적 개수로 기록한다. pending entry가 없거나 현재
  cursor가 entry 뒤 이미 이동한 경우는 안전한 미복원이다. 위치 확인·복원·검증 오류는 실패다.
- 모든 rollback 시도는 정확히 복원·미복원·실패 중 하나로 분류돼야 한다. 합계가 맞지 않으면 report
  자체가 잘못된 증거다.
- 입력 주입 실패나 cursor rollback 실패가 하나라도 있으면 `agent-session`은 `passed=true`가 될 수
  없다. 안전한 미복원이나 성공한 복원은 이후 인증 session과 destination input 증거가 충분하면
  통과를 막지 않는다.
- `krasis.lab.agent-session-check/v4`의 file 결과는 원본의 aggregate `destinationInput`을 포함하고,
  strict checker가 실패 집계와 rollback 합계를 다시 검증한다.
- `krasis.lab.phase0-check/v4`는 v4 pair checker 결과를 포함한다.
- report에는 token code, pointer 좌표·delta, session·epoch·lease, 주소, 인증서와 사용자 식별자를
  추가하지 않는다.

## 결과

단일 report와 pair checker만으로 input failure가 없었고 cursor 보상이 분류됐는지 확인할 수 있다.
이 결정이 도입한 shape는 이후 ADR 0012의 v8/v5 source/destination 분리로 확장됐다. 집계가 0이라는 사실은 실제 Windows가
실패를 정확히 보고했다는 증명은 아니므로 zero·partial·unknown 주입과 물리 cursor 경합은 실제 두
장비 failure-injection으로 별도 검증한다.

## 대안

- runtime error 문자열 저장: 입력값·환경 정보가 섞일 수 있고 안정적인 계약이 아니어서 기각한다.
- token별 실패 기록: 복구 분석에는 편하지만 키 코드와 입력 순서를 노출하므로 기각한다.
- rollback 실패만 기록: 최신 물리 이동 보호로 인한 정상 미복원과 실제 실패를 구분하지 못해 기각한다.
