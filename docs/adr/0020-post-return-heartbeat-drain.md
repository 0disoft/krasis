# ADR 0020: local return 뒤 retired inbound heartbeat를 drain한다

- 상태: Accepted
- 책임자: cherr
- 범위: runtime local-return tombstone과 같은 session의 늦은 heartbeat
- 후속 관계: ADR 0016의 input-only post-return drain을 heartbeat까지 보완한다

## 맥락

ADR 0016은 destination이 local return한 뒤 같은 retired inbound session의 늦은 input frame을 주입 없이
drain한다. 그러나 이미 전송 중이던 heartbeat가 뒤늦게 도착하면 runtime이 `LOCAL`이라는 이유만으로
liveness 오류가 됐고, 정상 return 직후 peer session failure와 불필요한 reconnect를 만들 수 있었다.

## 결정

- local return tombstone은 retired inbound session·epoch identity를 계속 보존한다.
- `LOCAL`에서 exact retired inbound session의 늦은 heartbeat는 성공적으로 drain한다.
- drain은 liveness를 재활성화하거나 deadline·last-observed state를 갱신하지 않고 runtime을 `LOCAL`로 유지한다.
- 다른 session의 heartbeat, tombstone이 없는 `LOCAL` heartbeat와 identity 불일치는 계속 오류다.
- 새 session activation은 기존 tombstone 규칙에 따라 fresh identity로만 가능하다.

## 결과

정상 return과 network에 남은 heartbeat의 순서 경쟁이 session failure로 승격되지 않는다. 반면 stale 또는
다른 peer generation이 `LOCAL` runtime을 되살릴 수 없으므로 fail-closed 경계는 유지된다.

## 대안

- 모든 `LOCAL` heartbeat 허용: stale session이 liveness를 오염시켜 기각한다.
- 모든 `LOCAL` heartbeat 오류 유지: 정상 return마다 reconnect churn을 만들 수 있어 기각한다.
- transport에서만 heartbeat 제거: 이미 수신 queue에 들어온 frame과 경쟁을 제거하지 못해 기각한다.

## 검증 gate

- exact retired session heartbeat는 성공하고 runtime은 `LOCAL`, liveness 비활성으로 남는다.
- 다른 session heartbeat는 오류이며 runtime state를 바꾸지 않는다.
- late input과 heartbeat가 섞여도 주입·재활성화·held input 잔류가 없어야 한다.
