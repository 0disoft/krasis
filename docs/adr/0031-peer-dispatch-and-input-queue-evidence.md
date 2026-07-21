# ADR 0031: 연결 종료와 입력 지연을 dispatch·queue·destination disposition으로 분해한다

- 상태: Experimental
- 책임자: cherr
- 범위: peer frame dispatch, source input queue, destination input disposition, Phase 0 report
- 관계: ADR 0030의 degraded focus release 정책은 유지한다. pointer coalescing이나 transport 교체를 결정하지 않는다.

## 맥락

2026-07-18 v20 r2 실기 실행에서 gram13은 승인된 boundary handoff `46`회를 request부터 revoke까지 모두
완료했다. exact focus restore는 `14`회 실패했지만 post-hide foreground는 `14`회 모두 unavailable로
분류됐고 capture retention 없이 session을 유지했다. 즉 ADR 0030의 terminal 조건 축소는 의도대로
작동했다.

그런데 양쪽 supervisor는 약 `2m` 동안 connection을 여덟 번 활성화했다. gram13은 여덟 번 모두
`peer-reader/read/remote_eof`, HP15는 일곱 번 모두 `peer-session/runtime/component_failed`로 기록했다.
양쪽 최대 heartbeat receive gap은 각각 `753ms`, `850ms`로 `3s` timeout보다 작았다. HP15는 pointer
`3851`, wheel `127`, button `4`개를 오류 없이 적용했지만 gram13 source는 pointer `4358`, wheel `127`,
button `4`개를 전송 성공으로 기록했다. wheel과 button은 보존됐고 pointer만 `507`개 차이가 났다.

이 차이는 return 뒤 늦게 도착해 retired stream에서 정상 폐기된 frame일 수도 있고, source queue backlog,
destination 거부 또는 frame dispatch failure일 수도 있다. 현재 counter만 보고 pointer를 합치거나 TCP를
바꾸면 원인을 숨기는 최적화가 된다.

## 결정

- peer reader가 frame을 읽은 뒤 처리에 실패하면 frame kind별 stable sentinel을 보존한다.
  - `peer-dispatch/control/failed`
  - `peer-dispatch/input/failed`
  - `peer-dispatch/heartbeat/failed`
- 기존의 더 구체적인 failure 원인, 특히 heartbeat timeout과 source capture failure는 위 generic dispatch
  분류보다 우선한다.
- source non-blocking queue는 다음 aggregate-only counter를 남긴다.
  - `enqueued`, `dequeued`, `discardedStale`, `maxDepth`
  - `residenceSamples`, `maxResidenceMicros`
- source hook과 destination input의 counter 갱신은 atomic 연산만 사용한다. report mutex를 hot path에서 잡지
  않는다. queue residence를 위해 source offer 시 clock을 한 번 읽는 비용은 v21 진단 비용으로 명시한다.
- destination runtime은 input frame마다 `received`를 한 번 기록하고 정확히 하나의 disposition을 남긴다.
  - `applied`: 실제 input port에 적용됨
  - `retiredDrained`: local return 뒤 같은 retired stream의 ordered late frame으로 검증됐지만 적용하지 않음
  - `rejected`: stream, action, ordering 또는 injection 처리 실패
- destination `applied`는 `destinationInput` action 합과 같아야 한다. disposition 합은 `received`와 같아야
  한다. `rejected>0`인 acceptance report는 실패다.
- source action 합은 `dequeued-discardedStale`보다 클 수 없다. queue residence는 monotonic aggregate가
  아니라 process clock의 duration sample이며 음수는 `0`으로 clamp한다.
- 좌표, key code, sequence, session ID, epoch, HWND와 raw 오류 문자열은 report에 넣지 않는다.
- v21 실기 결과 전에는 pointer coalescing, queue capacity 확대, transport 분리 또는 재연결 정책 변경을 하지
  않는다.

## 계약

- agent report: `krasis.lab.agent-session/v21`
- strict pair checker: `krasis.lab.agent-session-check/v18`
- Phase 0 aggregate: `krasis.lab.phase0-check/v17`

checker는 source queue conservation과 destination disposition conservation을 검증한다. source queue에는
report 종료 순간 아직 대기 중인 item이 있을 수 있으므로 `enqueued==dequeued`를 강제하지 않는다. 대신
`dequeued<=enqueued`, `discardedStale<=dequeued`, `residenceSamples==dequeued`를 요구한다.

## 결과

다음 실기 실행에서 체감 slow motion이 source queue의 큰 `maxDepth` 또는 `maxResidenceMicros`와 같이
나타나는지, source 전송과 destination 적용 차이가 `retiredDrained`으로 정확히 설명되는지, connection 종료가
어느 dispatch kind에서 시작되는지 판별할 수 있다. 숫자가 정상이라면 성능 추정이 아니라 Windows pointer
capture 또는 화면 경계 모델을 다시 의심해야 한다.

## 대안

- pointer move를 즉시 coalescing: 체감은 좋아질 수 있지만 loss와 정상 late drain을 구분할 evidence를 없애므로
  보류한다.
- queue capacity 확대: 지연을 줄이지 않고 backlog를 더 오래 보관할 수 있어 보류한다.
- 모든 EOF를 정상 reconnect로 간주: 실제 dispatch defect를 숨기므로 기각한다.
- 좌표와 sequence trace 저장: 진단은 쉬워지지만 입력 개인정보와 report 민감도가 커져 기각한다.

## 검증 gate

- unit test는 applied, retired-drained와 rejected를 각각 분류하고 합계 conservation을 검증해야 한다.
- source queue test는 accepted item의 enqueue/dequeue/depth/residence와 revoked generation의 stale discard를
  검증해야 한다.
- malformed control/input frame은 원래 decode sentinel과 frame-kind dispatch sentinel을 모두 보존해야 한다.
- gram13/HP15에서 menu와 drag가 없는 왕복을 최소 `20`회 수행하고 양쪽 v21 report를 보존한다.
- slow motion이 느껴진 회차를 포함해 source `maxDepth`, `maxResidenceMicros`, `discardedStale`와 destination
  `retiredDrained`, `rejected`, supervisor failure distribution을 함께 비교한다.
