# ADR 0014: Source pipeline 단계와 session failure component를 집계한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 `agent-session` report, supervisor와 strict checker

후속 결정: queue 거절 사유 분류와 return 경합 수정은 ADR 0015가 소유한다.

## 맥락

2026-07-15 gram13의 Precision Touchpad wheel은 독립 `capture`에서 physical wheel 505회로
관찰됐지만 두 차례 live agent report에서 source와 destination wheel이 모두 0이었다. 두 번째
실행은 60초 요청 중 약 20초 만에 `agent_session_failed`로 끝났고 HP15 receive gap도 3000ms
timeout 직전인 2889ms까지 증가했다. 기존 `sourceInput.wheels`는 network write에 성공한 action만
세므로 hook, mapper, queue 또는 sender 중 어디서 wheel이 사라졌는지 구분할 수 없었다.
supervisor도 모든 session failure를 하나로 합쳐 최초 실패 component를 보존하지 않았다.

## 결정

- `krasis.lab.agent-session/v9`은 aggregate `sourcePipeline.wheel`을 추가한다.
- `hookObserved`는 injected/tagged event를 제외한 physical wheel hook message 수다.
- `mapped`는 non-zero signed vertical/horizontal `InputAction`으로 정규화된 wheel 수다.
- `queueAccepted`와 `queueRejected`는 current source generation의 bounded queue offer 결과다.
- `sent`는 ordered transport write가 성공한 wheel 수이며 기존 `sourceInput.wheels`와 같아야 한다.
- strict evidence는 `mapped<=hookObserved`, `queueAccepted+queueRejected=mapped`,
  `sent<=queueAccepted`, `sent=sourceInput.wheels`를 요구한다. overflow를 피하는 차감식으로 검증한다.
- `supervisor.lastFailureComponent`는 `source-session`, `source-hook`, `boundary-orchestrator`,
  `return-controller`, `peer-session`, `peer-liveness`, `unknown` 중 하나만 기록한다.
- component error는 내부 cause를 wrapping해 cleanup 판단에는 보존하지만 report에는 component 이름만
  투영한다. 원문 오류, 입력 delta, 좌표, session/epoch/lease와 주소는 기록하지 않는다.
- `krasis.lab.agent-session-check/v6`와 `krasis.lab.phase0-check/v6`가 v9 shape와 count invariant를
  strict하게 검증한다.

## 결과

다음 한 번의 두 장비 실행으로 wheel 누락이 hook 관찰 전, mapping, queue offer, sender 또는 peer
component 중 어디에서 시작됐는지 좁힐 수 있다. 계측 observer는 payload를 받지 않고 action kind와
성공 여부만 집계한다. source pipeline count 자체는 제품 성능 SLO가 아니며 원인을 확정한 뒤에도
Phase 0 report의 회귀 증거로 유지한다.

`lastFailureComponent`는 최초 component 경계이지 root cause 문자열이 아니다. 예를 들어 remote가
먼저 연결을 닫으면 local에서는 `peer-session`으로 보일 수 있으므로 양쪽 report를 함께 비교해야 한다.

## 대안

- raw 오류 문자열 저장: 입력값, 주소와 운영 정보가 섞일 수 있고 stable contract가 아니어서 기각한다.
- wheel delta와 timestamp 저장: 진단력은 높지만 사용 패턴을 노출하므로 aggregate evidence에서 기각한다.
- 별도 wheel-only probe만 추가: live lease, queue, sender와 heartbeat 상호작용을 재현하지 못해 보조
  진단으로는 가능하지만 이번 결함의 주 증거로는 기각한다.
