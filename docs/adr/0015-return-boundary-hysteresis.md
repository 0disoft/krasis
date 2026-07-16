# ADR 0015: Destination 복귀에 히스테리시스를 적용하고 폐기된 source generation을 격리한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows destination return boundary, source queue generation과 Phase 0 evidence

후속 결정: local return 뒤 반대 방향 transport에 남은 exact input drain은 ADR 0016이 소유한다.

## 맥락

2026-07-15 gram13에서 HP15로 터치패드 제어를 넘긴 실험은 pointer 1861회와 button down/up 각
14회를 전송했지만 physical wheel 124회가 모두 source queue에서 거절됐다. gram13 report는
`hookObserved=124`, `mapped=124`, `queueAccepted=0`, `queueRejected=124`, `sent=0`이었고 HP15는
pointer 1638회와 button down/up 각 14회를 주입했지만 wheel은 0회였다. 두 장비 모두 2분 동안
connection을 6번 다시 획득하고 마지막 failure component를 `peer-session`으로 기록했다.

destination cursor는 source edge의 반대편 경계에서 1px 안쪽에 배치되지만 기존 return detector는
entry와 동시에 armed였다. entry 직후 남은 relative movement 하나가 바깥쪽이면 즉시 revoke될 수 있고,
사용자는 destination에 남은 비활성 cursor를 보면서 source의 local wheel을 조작하게 된다. 또한 revoke가
source queue의 이미 dequeue된 send와 겹치면 폐기된 generation의 send 오류가 worker 전체를 실패시켜
정상 return 뒤에도 TCP session을 닫고 supervisor reconnect를 유발할 수 있었다.

## 결정

- destination return detector는 entry 직후 return-ready가 아니다.
- source `RIGHT` entry는 destination 왼쪽 경계에서 안쪽으로, source `LEFT` entry는 destination 오른쪽
  경계에서 안쪽으로 configured rearm distance 이상 이동한 관찰을 한 번 거친 뒤에만 return-ready가 된다.
- return-ready가 된 뒤 exact entry edge에 다시 도달해야 return intent를 한 번 발행한다.
- `--rearm-distance`는 source boundary 재무장과 destination return 히스테리시스에 같은 값으로 적용한다.
- configured distance는 cursor entry inset보다 크고 현재 단일 화면 폭보다 작아야 한다. 화면 폭 이상이면
  destination return-ready가 영원히 false가 되므로 hook 시작 전에 거부한다.
- input held 때문에 revoke가 거부된 exact intent의 rearm은 이미 충족한 return-ready 상태를 유지한다.
- source sender가 action을 dequeue한 뒤 generation이 폐기되고 send가 실패한 경우 그 오류는 현재
  generation을 실패시키지 않는다. 새 generation action은 같은 worker에서 계속 전송한다.
- current generation의 send 실패와 queue overflow는 기존처럼 suppression을 즉시 해제하고 session을
  fail-closed로 종료한다.
- `krasis.lab.agent-session/v10`은 wheel queue rejection을 `rejectedInactive`, `rejectedInvalid`,
  `rejectedOverflow`로 분류한다. 세 값의 합은 `queueRejected`와 같아야 한다.
- strict checker schema는 `krasis.lab.agent-session-check/v7`, Phase 0 aggregate는
  `krasis.lab.phase0-check/v7`이다.

## 결과

entry 직후 잔여 delta로 제어권이 되튕기는 것을 막고, 정상 return과 in-flight send의 경합이 장기 peer
connection을 불필요하게 끊지 않는다. 사용자는 destination에서 rearm distance만큼 안쪽으로 이동한 뒤에야
source 방향 경계로 복귀할 수 있다. 32px Phase 0 기본값에서는 의도한 복귀에 작은 추가 이동이 필요하지만
1px 흔들림보다 안정적인 제어권이 우선이다.

실제 gram13 Precision Touchpad가 v10에서 `queueAccepted>0`, `sent>0`이고 HP15에서
`destinationInput.wheels>0`인지, 양쪽 `supervisor.activeCount=1`, `sessionFailures=0`인지 확인하는 두 장비
재실험은 여전히 필요하다.

## 대안

- inactive wheel을 다음 lease까지 보관: stale 입력을 다른 session에 전달할 수 있어 기각한다.
- inactive wheel도 source에서 항상 억제: 로컬 제어권인데 scroll을 잃는 fail-closed UX라 기각한다.
- cursor entry inset만 크게 증가: 초기 위치는 바꾸지만 return detector와 generation race를 해결하지
  못하므로 보조 조정으로만 남긴다.
