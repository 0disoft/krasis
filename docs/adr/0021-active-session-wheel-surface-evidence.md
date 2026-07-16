# ADR 0021: active session에서 wheel 입력 표면과 forwarding을 함께 계측한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 Windows source wheel 진단과 report gate
- 관계: ADR 0013의 hook-owned wheel payload를 유지하고 ADR 0019의 standalone 입력 표면 진단을 active session으로 확장한다

## 맥락

standalone `wheel-surface`에서 같은 조작 구간의 physical hook wheel과 Raw Input wheel이 각각
`77/77`로 관찰돼 `HOOK_AND_RAW`가 나왔다. 이 결과는 Raw Input에 wheel flag가 없다는 과거 가설을
반박한다. 하지만 두 값은 aggregate count다. 같은 OS event를 hook callback과 Raw packet 사이에서
correlate하지 않으므로, `77/77`이라는 숫자만으로 두 표면이 정확히 같은 event를 나타낸다고 증명할 수
없다.

`HOOK_AND_RAW`는 split suppression이나 send ownership의 근거도 아니다. Raw Input이 payload를 만들고
hook이 억제만 맡게 바꾸면 두 비동기 callback 사이의 exact event identity, ordering과 실패 처리를 새로
정의해야 한다. standalone report에는 active lease, suppression, source queue와 transport send가 없어서
그 경계를 검증하지 못한다.

## 결정

- `WH_MOUSE_LL`은 계속 vertical/horizontal wheel payload의 유일한 mapper이자 sender ingress다.
  Raw Input은 wheel payload를 만들거나 보내지 않는다.
- `krasis.lab.agent-session/v12`의 `sourcePipeline.wheel`은 기존 `hookObserved`,
  `synthesizedObserved`, `mapped`, `queueAccepted`, `queueRejected`, `rejectedInactive`,
  `rejectedInvalid`, `rejectedOverflow`, `sent`를 유지한다.
- 같은 object에 active source session 동안의 독립 aggregate evidence를 추가한다.
  - `rawObserved`: 유효하고 tag가 없으며 delta가 non-zero인 Raw vertical/horizontal wheel axis 관찰 수
  - `activeHookMoves`: tag 없는 physical `WH_MOUSE_LL` move callback 수
  - `activeHookButtons`: tag 없는 physical `WH_MOUSE_LL` button callback 수
- 세 counter는 active source session 구간만 센다. 좌표, delta, timestamp, device identity와 event
  trajectory는 저장하지 않는다.
- `rawObserved`, `activeHookMoves`, `activeHookButtons`, `hookObserved`와
  `synthesizedObserved`는 서로 독립인 aggregate다. counter 사이 exact equality나 한 OS event의
  상관관계를 요구하지 않고, Raw Input의 suppression 또는 payload ownership을 암시하지 않는다.
- report에서 `rawObserved>0`인데 `sent==0`이면 `passed=false`와 stable failure code
  `raw_wheel_observed_without_forwarding`을 남긴다. strict checker도 같은 code로 file을 실패시킨다.
- strict checker schema는 `krasis.lab.agent-session-check/v9`, Phase 0 aggregate schema는
  `krasis.lab.phase0-check/v9`다.

## 결과

다음 두 장비 실행은 “Raw wheel이 OS에 없었나”와 “active hook 또는 forwarding 경로에서 사라졌나”를
한 report 안에서 가른다. `rawObserved>0`, `activeHookMoves>0` 또는 `activeHookButtons>0`인데 wheel
`hookObserved=0`이면 active low-level hook은 살아서 mouse callback을 받았지만 wheel callback만 보이지
않은 경우를 좁힐 수 있다. `rawObserved>0`과 `sent=0`은 owner 전환 신호가 아니라 현재 hook-owned
forwarding 실패를 재현한 안전한 중단 gate다.

이 계측만으로 Raw/hook 중복, 순서, 동일 event 여부나 일반 mouse의 한 notch당 단일 적용을 증명하지
않는다. payload ownership 변경은 별도 event correlation과 suppression/send 실패 계약이 있는 새 ADR
없이는 허용하지 않는다.

Raw Input은 비동기 queue이므로 `rawObserved`는 packet 발생 시점이 아니라 dequeue 시점의 active source
session을 기준으로 센다. 직전 LOCAL 또는 INBOUND 구간의 늦은 packet이 다음 outbound 활성화 뒤 처리되면
보수적인 false failure가 날 수 있다. 이 진단 report는 안전 성공을 과대평가하지는 않지만 실패 원인을
확정하기 전 전환 직후 backlog 가능성을 배제해야 한다.

## 대안

- Raw Input으로 wheel payload ownership 즉시 이전: standalone `HOOK_AND_RAW`가 active suppression,
  queue와 send 경계를 검증하지 않아 기각한다.
- hook suppression과 Raw send를 분리하고 aggregate count로 dedupe: 같은 OS event identity가 없어
  누락과 중복을 구분할 수 없으므로 기각한다.
- 기존 hook counter만 유지: Raw wheel이 관찰되는 동안 hook-owned forwarding이 0인 실패를 active
  session에서 직접 구분하지 못해 기각한다.

## 검증 gate

- gram13 Precision Touchpad와 일반 mouse를 각각 새 `agent-session/v12` report로 수집한다.
- 경계를 넘은 뒤 pointer를 움직여 active hook health를 남기고 1초간 기다린 다음 wheel을 조작한다.
- active 구간에서 `rawObserved`, `activeHookMoves`, `activeHookButtons`, 기존 wheel pipeline과
  destination wheel을 함께 비교한다.
- `rawObserved>0 && sent==0` report가 `raw_wheel_observed_without_forwarding`으로 실패하는지 확인한다.
- 일반 mouse는 한 notch가 destination에 한 번만 적용되는지 별도 수동 검증한다.
