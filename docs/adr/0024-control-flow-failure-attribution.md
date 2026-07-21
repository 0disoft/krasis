# ADR 0024: boundary handoff 단계와 실제 session failure origin을 집계한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 `agent-session` control flow, supervisor failure attribution와 strict checker
- 관계: ADR 0014의 `lastFailureComponent`를 대체하고 ADR 0015·0023의 동작 정책은 유지한다

## 맥락

2026-07-17 S-B/S-A v14 실행에서 source boundary intent는 `22`, destination entry는 `18`, source capture
activation은 `11`이었다. 사용자는 일부 왕복이 즉시 되지 않다가 다시 되는 현상을 관찰했다. 양쪽
supervisor는 session failure `7`건과 `lastFailureComponent=peer-session`을 남겼지만 이 값만으로는
boundary admission, request write, remote grant, local grant apply, source activation과 capture activation 중
어디에서 한 세대가 멈췄는지 구분할 수 없었다.

기존 component 값은 동시에 종료되는 goroutine 중 먼저 관찰된 경계를 기록했다. source activation 실패가
연결을 닫으면 peer reader도 실패하므로 실제 source 원인보다 뒤따른 read 오류가 먼저 보일 수 있었다.
`22-18=4`, `18-11=7`이라는 aggregate 차이는 유력한 위치를 가리키지만 세대별 인과를 증명하지 않는다.

## 결정

- `krasis.lab.agent-session/v15`는 payload, 좌표와 timestamp 없이 aggregate `controlFlow`를 추가한다.
- `boundary`는 queued intent를 admitted, not-ready drop과 admission failure로 분류한다. 현재 not-ready
  drop과 32px rearm 정책은 바꾸지 않는다. source action이 실제 전송된 report에서는 세 분류의 합이
  queued intent와 정확히 같아야 한다. destination evidence만 완전하고 반대 방향 boundary가 종료 직전
  queue에 남은 report는 기존 one-way evidence 계약대로 미분류 잔여를 허용한다.
- outbound는 request write, grant receive/apply, source session activation, capture activation과 revoke
  receive를 센다. write 뒤 응답을 받기 전에 연결이 끊긴 경우는 성공으로 추정하지 않고
  `OutcomeUnknown`으로 분류한다.
- inbound는 request receive, return arm, grant write, entry commit과 revoke write를 센다. grant write가
  완료된 뒤에만 entry를 commit한다.
- transport는 local connection interrupt와 그 뒤 peer reader가 깨어난 횟수를 센다. local interrupt의
  원인 error는 connection close로 발생한 read error와 함께 보존한다.
- supervisor는 `lastFailureComponent` 대신 `firstFailure`와 `failureCounts`를 기록한다. 각 failure는
  stable `component`, `phase`, `code` triple만 가지며 raw error 문자열은 report에 넣지 않는다.
- failure attribution은 source/capture activation, outbound state transition, semantic write, local-interrupt
  read, remote EOF·truncated frame·일반 I/O, heartbeat timeout 순으로 실제 typed cause를 우선한다. 해당
  cause가 없을 때만 기존 runtime component를 fallback으로 쓴다.
- `failureCounts`의 triple은 중복될 수 없고 count 합은 `supervisor.sessionFailures`와 정확히 같아야 하며
  `firstFailure`는 그 분포에 포함돼야 한다. control-flow 단계 합계도 기존 source/destination/capture
  aggregate와 맞아야 한다.
- 단계 실패가 한 건이라도 있으면 `control_flow_failure_observed`, not-ready drop이 있으면
  `boundary_not_ready_drop_observed`로 report를 실패시킨다. 합계 자체가 모순이면
  `invalid_control_flow_evidence` 또는 `invalid_failure_evidence`다.
- strict pair checker는 `krasis.lab.agent-session-check/v12`, Phase 0 aggregate는
  `krasis.lab.phase0-check/v12`를 사용한다.

## 결과

다음 실제 두 장비 실행은 `queued→admitted→request write`, `request receive→grant write→entry commit`,
`grant receive→apply→source activate→capture activate` 중 어느 구간에서 손실 또는 실패가 생겼는지
보고서만으로 좁힐 수 있다. source가 연결을 먼저 끊은 경우에도 `peer-reader/read/after_local_interrupt`가
원래 source failure를 덮지 않는다.

이 계측은 per-generation trace가 아니다. 여러 왕복의 aggregate를 합치므로 서로 다른 세대의 counter를
임의로 짝지어 단일 사건을 재구성할 수 없다. v15 결과로 단계가 좁혀진 뒤에도 재시도, boundary debounce,
rearm distance나 focus 정책을 바꾸려면 별도 ADR과 실제 장비 검증이 필요하다.

## 대안

- session·epoch별 event timeline 저장: 인과는 강하지만 timestamp와 식별자 보존 범위가 커지고 Phase 0
  privacy 계약을 넓히므로 보류한다.
- 기존 `lastFailureComponent`만 유지: secondary peer read symptom이 실제 source cause를 덮어 원인 분리가
  되지 않으므로 기각한다.
- not-ready boundary를 즉시 retry: 진단 전에 동작을 바꾸면 intermittent defect를 숨길 수 있어 기각한다.
- raw error 문자열 저장: 주소, 운영 상태와 내부 값이 섞일 수 있고 stable contract도 아니어서 기각한다.

## 검증 gate

- unit test는 boundary admission 세 분류, request/grant/entry 순서와 local interrupt cause 보존을 검증한다.
- supervisor test는 source/capture typed cause가 뒤따른 peer read 오류보다 우선하는지 검증한다.
- report test는 control-flow conservation, `uint64` overflow, failure count 합과 stable triple allowlist를
  검증한다.
- 실제 S-A/S-B 실행은 양쪽 v15 report와 guard를 남기고 intermittent 왕복을 반복한다. 결과가 clean이어도
  하드웨어 결함이 사라졌다고 단정하지 않고, boundary drop과 stage failure가 0인지 먼저 확인한다.
