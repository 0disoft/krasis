# ADR 0027: local interaction을 차단하고 destination entry를 2단계로 확정한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows source boundary admission, ordered control protocol, lease state machine, Phase 0 evidence
- 관계: ADR 0009의 cursor prepare/rollback을 wire commit까지 확장하고 ADR 0023의 touchpad capture owner를 유지한다. ADR 0025에서 후속으로 보류한 two-phase handoff를 채택한다. capture foreground fallback은 ADR 0028이 소유하며 이 commit/abort 경계를 바꾸지 않는다.

## 맥락

2026-07-17 gram13의 launcher log `EDIT` control에서 text를 drag한 뒤 왼쪽 경계를 통과하려는 실행은
boundary request, grant와 source session apply를 10번 완료했지만 touchpad capture activation은 3번만
성공하고 7번 실패했다. `blockedLocalMenu`는 0이었다. Windows foreground thread의 menu flag만 확인하는
ADR 0025 guard는 `GUITHREADINFO.hwndCapture`, `GUI_INMOVESIZE`와 현재 눌린 input을 분류하지 않았다.

기존 wire 순서는 destination이 `LeaseGrant`를 쓴 직후 cursor entry와 `INBOUND_ACTIVE`를 확정했다. 이후
source capture activation이 실패하면 destination은 이미 진입했지만 source는 input을 보낼 수 없는 half-entry가
됐다. menu, drag, move/size loop를 계속 열거하는 방식만으로는 admission 검사 직후 foreground 상태가 바뀌는
경합을 제거할 수 없다.

## 결정

### Local admission

- source boundary gate는 request와 outbound identity 생성 전에 foreground `GUITHREADINFO`와 async key state를
  한 번 검사한다.
- menu flag는 `blockedLocalMenu`로 분류한다.
- `hwndCapture != 0`, `GUI_INMOVESIZE` 또는 `hwndMoveSize != 0`, 현재 눌린 key나 mouse button은 각각
  `blockedMouseCapture`, `blockedMoveSize`, `blockedPressedInput`으로 분류한다. 세 counter의 합은
  `blockedLocalInteraction`과 같아야 한다.
- 차단된 intent는 request를 보내거나 identity를 만들지 않는다. 조회 실패와 정의되지 않은 decision은
  `admissionFailed`로 fail-closed 처리한다.
- Krasis는 local capture를 강제로 해제하거나 menu와 drag를 자동 취소하지 않는다. 사용자가 input을 놓고
  cursor를 안쪽으로 이동해 boundary detector를 rearm해야 한다.

### Two-phase entry

- control payload version은 `3`이며 ordered wire command는 `LeaseRequest`, `LeaseGrant`, `LeaseReject`,
  `LeaseCommit`, `LeaseAbort`, `LeaseRevoke`다.
- destination은 승인된 request에서 return detector와 cursor entry를 prepare하고 runtime을
  `INBOUND_PENDING`으로 전이한 뒤 `LeaseGrant`를 쓴다. 이 시점에는 cursor entry를 commit하지 않고 input도
  받지 않는다.
- source는 exact session, lease와 epoch의 grant를 적용한 뒤 source session과 touchpad capture window를 모두
  활성화한다. 준비가 성공하면 같은 identity의 `LeaseCommit`을 먼저 쓰고, ordered connection에서 뒤따르는
  input은 commit보다 앞설 수 없다.
- source activation이 실패하면 `LeaseAbort`를 쓰고 source runtime과 suppression을 `LOCAL`로 되돌린다.
  destination은 exact abort에서 prepared entry를 rollback하고 return detector를 disarm한다.
- destination은 exact commit을 받은 뒤에만 `INBOUND_ACTIVE`와 cursor entry를 commit한다. commit, abort의
  identity mismatch와 duplicate transition은 protocol failure다.
- grant write 실패, commit/abort write outcome unknown, EOF 또는 session 종료는 pending entry를 bounded
  cleanup context에서 rollback한다. blind retry는 하지 않는다.

### Evidence와 계약

- capture activation failure는 `captureActivationSetupFailed`, `captureActivationApiRejected`,
  `captureActivationVerificationLost` 중 정확히 하나로 분류한다.
- control-flow evidence는 commit/abort write 결과, destination commit/abort receive,
  `entryPrepareAttempted`와 성공한 prepare, commit/rollback 시도를 포함한다. prepare 자체가 실패해도 cursor가
  일부 이동했을 수 있으므로 모든 prepare 시도는 commit 또는 rollback 시도로 보존돼야 한다.
- strict JSON 계약은 `krasis.lab.agent-session/v21`, pair checker는
  `krasis.lab.agent-session-check/v18`, Phase 0 aggregate는 `krasis.lab.phase0-check/v17`다.

## 상태 전이

| 현재 상태 | event | 다음 상태 | 주요 effect |
| --- | --- | --- | --- |
| `LOCAL` | `INBOUND_LEASE_PREPARED` | `INBOUND_PENDING` | cursor와 return detector는 prepared, input 비활성 |
| `INBOUND_PENDING` | `INBOUND_LEASE_COMMITTED` | `INBOUND_ACTIVE` | inbound input 활성, cursor entry commit |
| `INBOUND_PENDING` | `INBOUND_LEASE_ABORTED` | `LOCAL` 수렴 | prepared cursor rollback, return detector disarm |
| `PENDING` 또는 `OUTBOUND_ACTIVE` | `OUTBOUND_LEASE_ABORTED` | `LOCAL` 수렴 | source suppression과 capture 해제 |

`FAULT_DETECTED`와 connection cleanup도 `INBOUND_PENDING`을 `LOCAL`로 수렴시키며 prepared cursor rollback은
`LeaseResponder`가 소유한다.

## 결과

local drag나 menu 때문에 source capture가 준비되지 않더라도 destination cursor는 먼저 진입하지 않는다.
단순 admission guard와 wire commit이 서로 다른 경합 창을 막는다. report는 사용자가 button을 누른 상태였는지,
foreground control이 mouse capture를 소유했는지, Windows가 foreground API를 거부했는지를 분리할 수 있다.

## 대안

- `SetForegroundWindow` retry: 사용자의 foreground를 반복해서 빼앗고 half-entry 시간을 늘려 기각한다.
- `ReleaseCapture` 강제 호출: 다른 process의 gesture를 안전하게 해제할 수 없고 사용자 drag를 파괴해 기각한다.
- admission guard만 확장: 검사 직후 상태가 바뀌는 TOCTOU 경합이 남아 기각한다.
- commit 없이 source failure 때 connection close만 사용: destination cleanup은 가능하지만 정상 connection을 매번
  버리고 실패 원인을 숨겨 `LeaseAbort`를 명시적으로 둔다.

## 검증 gate

- menu, mouse capture, pressed input, move/size decision은 identity와 request 전에 각각 차단된다.
- control payload v3는 commit과 abort의 exact identity 및 lease 필수 조건을 round-trip 검증한다.
- destination은 grant 뒤 `INBOUND_PENDING`과 entry 0, commit 뒤 `INBOUND_ACTIVE`와 entry 1이어야 한다.
- source capture 실패는 abort를 쓰고 양쪽 runtime과 cursor를 `LOCAL`로 rollback해야 한다.
- pending 상태의 EOF와 malformed commit은 cursor와 return detector를 남기지 않아야 한다.
- 전체 Go test, vet, lab·launcher build와 두 장비 실기 검증을 통과해야 한다.
