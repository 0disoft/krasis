# ADR 0012: Agent evidence는 송신과 수신을 장비별로 분리한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 `agent-session` report와 strict pair checker
- 후속 결정: source pipeline 진단 shape는 ADR 0014의 v9/v6 계약이 확장한다

## 맥락

`agent-session/v7`은 각 장비가 destination input을 받았을 때만 `passed=true`가 될 수 있었다.
따라서 한 방향만 실행한 정상 실험에서는 source 장비가 boundary를 넘고 입력을 성공적으로
전송했어도 `destinationInput=0`이라는 이유로 실패했다. 2026-07-15 S-B에서 touchpad로 S-A를
제어한 실험은 S-A에 entry 10회와 pointer move 1794회가 기록됐지만, source 전용 S-B report는
`no_destination_input_observed`로 실패했다. 이는 입력 경로 실패가 아니라 증거 모델의 양방향 가정이다.

## 결정

- `krasis.lab.agent-session/v8`은 aggregate `sourceInput`을 추가한다.
- `sourceInput`은 bounded queue에 실제로 들어간 boundary intent 수와 transport write에 성공한
  key/button down/up, pointer move, wheel 수만 기록한다.
- boundary detector가 intent를 만들었더라도 handoff queue가 가득 차서 버린 intent는 세지 않는다.
  source action도 frame write가 성공한 뒤에만 센다.
- 단일 report는 liveness와 안전 조건을 만족하고, 완전한 source evidence 또는 완전한 destination
  evidence 중 하나가 있으면 통과할 수 있다.
- source evidence는 boundary intent 하나 이상과 성공적으로 전송한 action 하나 이상이다.
  destination evidence는 entry, return arm과 성공적으로 적용한 action이 각각 하나 이상이다.
- `krasis.lab.agent-session-check/v5`는 두 report가 개별적으로 유효한지 검사한 뒤 pair 전체에서
  source evidence와 destination evidence가 각각 최소 한 번 존재하는지 검사한다.
- source와 destination의 exact action count 일치는 요구하지 않는다. session 경계, return과
  취소 시점 때문에 양쪽 집계가 달라도 전달 경로가 정상일 수 있다.
- `krasis.lab.phase0-check/v5`는 v5 pair checker 결과를 포함한다.
- source evidence에는 key/button code, pointer 좌표·delta, session·epoch·lease와 장치 식별자를
  기록하지 않는다.

## 결과

한 방향만 실행한 실제 두 장비 실험도 source report와 destination report를 합쳐 정상 판정할 수 있다.
`sourceInput.boundaryIntents`는 화면 끝에 닿지 않았다고 느꼈는데 전환된 현상을 다음 실험에서 entry
횟수와 비교할 수 있는 최소 집계 자료가 된다. 다만 intent 수만으로 detector가 너무 일찍 발화했는지
판단할 수는 없으므로 좌표나 원시 packet을 저장하는 대신 별도 bounded shadow 진단을 사용한다.

v7/v4 report는 strict v8/v5 checker 입력으로 자동 승격하지 않는다. 기존 보고서는 당시 실행의
원본 증거로 보존하고, 새 판정이 필요하면 같은 build의 새 executable로 다시 수집한다.

## 대안

- source 장비의 `destinationInput=0`을 특별히 무시: 송신 자체가 없었던 빈 session도 통과할 수 있어
  기각한다.
- 두 방향을 반드시 한 실행에서 수행: 사용성 검증에는 유용하지만 종료 시점과 사람의 조작 순서에
  따라 정상 단방향 증거까지 버리므로 checker의 필수 조건으로 두지 않는다.
- source action payload나 좌표 저장: 조기 경계 진단은 쉬워지지만 입력 내용과 사용 패턴을 노출하므로
  aggregate report에서는 기각한다.
