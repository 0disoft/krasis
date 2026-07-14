# ADR 0009: destination cursor entry를 grant 전 보상 가능한 효과로 둔다

- 상태: Accepted
- 책임자: cherr
- 범위: inbound lease request와 Windows cursor adapter

## 맥락

destination은 inbound lease를 승인하기 전에 source edge 반대쪽으로 cursor를 옮긴다. cursor 이동 뒤
`GrantInbound` 또는 grant frame write가 실패하면 runtime은 `LOCAL`로 복구되지만 cursor만 화면 끝에
남을 수 있다. 반대로 실패했다고 무조건 예전 좌표를 덮어쓰면 entry 뒤 사용자가 직접 움직인 최신
cursor 위치를 망가뜨린다.

## 결정

- cursor adapter 계약을 `PrepareEntry`, `CommitEntry`, `RollbackEntry` 세 단계로 둔다.
- `PrepareEntry`는 이동 전 좌표를 snapshot하고 entry 좌표로 이동한 뒤 실제 좌표를 다시 읽어
  성공을 확인한다. 같은 adapter에는 동시에 하나의 pending entry만 허용한다.
- destination 순서는 return detector arm, cursor prepare, inbound runtime grant, grant frame write,
  cursor commit이다. grant frame write가 성공하기 전까지 cursor 이동은 미확정 효과다.
- prepare 이후 어느 단계에서든 실패하면 return detector를 disarm하고, 호출 context의 취소를
  상속하지 않는 bounded cleanup context로 rollback을 시도한다. 원래 실패와 rollback 실패는 함께
  보존한다.
- rollback 직전에 현재 cursor가 prepare가 확인한 entry 좌표와 같은 경우에만 snapshot 좌표로
  복원한다. 이미 다른 좌표라면 사용자 또는 다른 입력이 소유권을 가져간 것으로 보고 덮어쓰지 않는다.
- entry 뒤 실제 좌표를 읽지 못해 소유권을 확인할 수 없으면 무조건 복원하지 않고 불확실한 상태
  오류를 반환한다.
- 성공 evidence의 `entries`는 prepare 시점이 아니라 commit 시점에만 증가한다.

## 결과

권한 승인 뒤 runtime grant나 network write가 실패해도 destination cursor를 가능한 범위에서 원위치로
복구한다. 사용자 cursor가 entry 뒤 움직였다면 오래된 snapshot으로 덮어쓰지 않는다. 보상 실패는
원래 장애를 가리지 않고 session 실패에 함께 남는다.

Windows cursor API에는 compare-and-set이 없으므로 rollback의 현재 좌표 확인과 실제 복원 호출 사이에
아주 짧은 경쟁 창은 남는다. 실제 두 장비에서 물리 mouse 이동과 grant failure를 겹치는 실험 전에는
이 결정만으로 완전한 cursor 소유권을 증명하지 않는다.

## 대안

- 실패마다 무조건 snapshot 복원: 사용자의 최신 cursor 이동을 덮어쓸 수 있어 기각한다.
- cursor를 grant frame write 뒤 이동: source가 grant를 받은 직후 destination entry가 아직 준비되지
  않은 순서 구멍이 생겨 기각한다.
- 별도 ready/commit network round trip 추가: 더 강한 합의가 필요하다는 실제 장비 증거가 나오기 전에는
  protocol 복잡도에 비해 이득이 작아 보류한다.
