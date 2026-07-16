# ADR 0016: Local return 뒤 exact late input을 주입 없이 drain한다

- 상태: Accepted
- 책임자: cherr
- 범위: session runtime input ordering, peer connection cleanup과 supervisor retry

## 맥락

ADR 0015 적용 뒤 gram13→HP15 실험은 destination pointer 878회와 button down/up 각 21회를 정상
주입했지만 약 47초 만에 connector가 `agent_session_failed`로 종료됐다. 양쪽 heartbeat 최대 gap은
800ms 이하로 3000ms timeout과 거리가 있었지만 connector는 `activeCount=2`, `sessionFailures=1`,
listener는 `activeCount=2`, `sessionFailures=2`와 `peer-session`을 기록했다.

return controller는 destination runtime을 먼저 `LOCAL`로 정리한 뒤 반대 방향 transport에 revoke를
쓴다. source가 revoke를 받기 직전에 이미 source→destination writer에 들어간 input frame은 destination의
local cleanup 뒤 도착할 수 있다. 기존 runtime은 `LOCAL`에서 받은 모든 input을 wrong-direction protocol
failure로 처리해 connection을 닫았다. 이 Close 오류가 이미 session error에 포함됐는데 supervisor는 이를
독립적인 cleanup 실패로 다시 분류해 두 번째 connection에서 즉시 terminal 종료할 수 있었다.

## 결정

- 성공한 `RevokeLocalReturn`은 방금 폐기한 inbound `sessionID`, `epoch`, `NextSequence`만 memory에
  retired stream tombstone으로 남긴다.
- runtime이 `LOCAL`인 동안 tombstone의 exact session·epoch와 다음 연속 sequence가 일치하는 input은
  action을 주입하지 않고 sequence만 전진시켜 성공적으로 drain한다.
- 다른 session, 다른 epoch, duplicate, sequence gap, exhausted stream과 invalid action은 계속 오류다.
- 새 inbound grant가 성공하면 이전 tombstone을 즉시 폐기한다. active lease에서 session이나 epoch가
  다른 input은 기존처럼 fail-closed다.
- peer revoke, generic failure와 outbound transition은 retired stream을 새 권한으로 승격하지 않는다.
- supervisor가 connection을 닫을 때 반환된 오류가 session error에 이미 포함돼 있으면 그 connection의
  owner가 Close를 시도한 것으로 보고 `LOCAL` cleanup 뒤 backoff/retry를 계속한다.
- session error에 포함되지 않은 독립적인 connection Close 실패, partial acquire Close 실패와 source
  Close 실패는 기존처럼 terminal이다.

## 결과

정상 return과 in-flight input의 반대 방향 경합이 장기 peer connection을 끊지 않는다. 늦은 입력은 화면에
적용되지 않고 exact ordered tombstone에서만 소비되므로 새 lease나 다른 peer session으로 넘어가지 않는다.
Close 오류를 무조건 무시하지 않고 session owner가 이미 관찰한 동일 오류에만 retry를 허용한다.

실제 두 장비에서 한 connection으로 여러 번 왕복하며 `supervisor.activeCount=1`,
`sessionFailures=0`이 유지되는지와 remote Precision Touchpad wheel이 destination에 주입되는지는 별도
재실험이 필요하다.

## 대안

- `LOCAL`의 모든 input 무시: 인증된 연결이라도 다른 session·epoch 공격과 protocol bug를 숨겨 기각한다.
- revoke를 보내기 전 grace sleep: transport 순서를 보장하지 않고 hook callback을 지연시켜 기각한다.
- 모든 Close 오류 무시: 소유되지 않은 socket과 partial acquire 실패를 숨길 수 있어 기각한다.
