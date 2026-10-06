# ADR 0025: local Windows menu loop에서는 원격 전환을 요청하지 않는다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows source boundary admission, Phase 0 control-flow evidence와 failure code 우선순위
- 관계: ADR 0024의 boundary 분류를 확장한다. ADR 0023의 touchpad capture owner는 유지한다. 여기서 보류한 two-phase handoff는 ADR 0027이 채택했다.

## 맥락

2026-07-17 S-B/S-A v15 통제 실행에서 gram13의 local context menu를 열지 않았을 때는 14번의
boundary intent가 모두 request, grant, source capture activation과 revoke까지 완료됐다. 같은 build와
장비에서 gram13 context menu를 연 채 8번 시도하자 request와 remote entry는 모두 8번 완료됐지만 source
capture activation은 8번 모두 실패했고 revoke는 한 번도 완료되지 않았다. HP15는 entry 8건을 commit한
뒤 input 0건, revoke 0건을 기록했다. 양쪽 crash guard는 clean이었다.

따라서 메뉴가 열린 실행의 `insufficient_liveness_evidence`는 최초 원인이 아니었다. Windows foreground
thread의 modal menu loop가 touchpad-capable capture window의 foreground activation을 막았고, 이미 entry를
commit한 destination은 source가 준비되지 않은 채 연결 종료를 기다렸다. 네트워크 재시도나 rearm distance
조정으로 이 순서를 고치면 원인을 숨길 뿐이다.

Microsoft의 [`GetGUIThreadInfo`](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getguithreadinfo)
계약은 thread ID `0`에서 foreground thread 정보를 반환하며 `GUI_INMENUMODE`, `GUI_SYSTEMMENUMODE`,
`GUI_POPUPMENUMODE`로 활성 menu 상태를 노출한다.

## 결정

### Current bounded dismissal policy

The implementation now attempts one bounded cancellation of an idle native menu
and rechecks the same foreground owner before crossing. An unknown mouse capture
is not a menu: cancel it only after a local right-button release on that same
foreground owner, when the captured HWND is a distinct owned tool/nonactivating
popup. Any other mouse button or keyboard event clears that gesture evidence.
Dragging, held input, an unowned overlay and an ordinary captured control remain
blocked without a cancellation message. Unrecognized custom menus may require
manual dismissal; they must not justify canceling another application's mode.
These rules supersede the historical blanket no-dismiss policy below.

- source boundary orchestrator는 outbound readiness 확인 뒤 identity 생성과 request write보다 먼저 Windows
  foreground GUI thread의 menu 상태를 검사한다.
- 세 menu flag 중 하나라도 켜져 있으면 해당 intent를 `blockedLocalMenu`로 분류하고 identity를 만들거나
  peer request를 보내지 않는다. destination cursor entry도 발생하지 않는다.
- menu 상태 조회 자체가 실패하면 허용하지 않는다. `admissionFailed`로 분류하고 session을 fail-closed로
  종료한다.
- Krasis는 local menu를 자동으로 닫거나 foreground를 반복해서 빼앗거나 같은 intent를 blind retry하지
  않는다. 차단된 intent는 기존 boundary detector의 rearm 규칙을 따르므로 사용자가 cursor를 안쪽으로
  움직인 뒤 다시 경계에 닿아야 재시도된다.
- `blockedLocalMenu`는 의도된 안전 차단이며 stage failure나 not-ready drop으로 세지 않는다. boundary
  conservation은 `admitted + droppedNotReady + blockedLocalMenu + admissionFailed = queued`를 사용한다.
- strict JSON 계약은 `krasis.lab.agent-session/v16`, pair checker는
  `krasis.lab.agent-session-check/v13`, Phase 0 aggregate는 `krasis.lab.phase0-check/v13`으로 올린다.
- report의 대표 failure code는 구조적으로 유효한 control-flow/failure evidence와 stage failure를 먼저
  평가한 뒤 liveness 부족을 평가한다. capture activation 실패가 heartbeat 표본 부족으로 가려지지 않게
  한다.

## 결과

local context menu가 열린 동안 경계에 닿아도 상대 장비 cursor가 먼저 진입하지 않는다. menu를 닫고
안쪽으로 rearm한 뒤 다시 경계에 닿으면 정상 handoff를 시도한다. menu query 실패도 원격 half-entry보다
local failure로 수렴한다.

이 결정은 현재 결함의 재현 조건을 막지만 handoff 자체를 원자적으로 만들지는 않는다. menu 조회 직후
menu가 새로 열리거나 capture activation이 다른 이유로 실패하는 경합은 남는다. source capture readiness를
destination entry commit보다 앞세우는 prepare/commit 또는 rollback ACK 기반 two-phase handoff는 별도 ADR과
protocol 변경으로 다룬다.

## 대안

- context menu를 자동 dismiss: 사용자 UI 상태를 파괴하고 다른 modal surface에는 일반화되지 않아 기각한다.
- `SetForegroundWindow` 반복 retry: foreground 규칙을 우회하려는 timing loop가 되고 destination half-entry
  시간을 늘리므로 기각한다.
- destination entry 뒤 source activation 실패 시 cursor rollback만 추가: 현재 generation과 사용자 cursor
  이동을 구분할 protocol ACK가 없어 안전하지 않으므로 보류한다.
- 즉시 two-phase handoff 도입: 최종 방향은 더 강하지만 control protocol과 recovery semantics가 함께
  바뀌므로 이번 재현 결함의 bounded fix 범위를 넘는다.

## 검증 gate

- unit test는 세 menu flag 각각이 identity 생성과 `BeginBoundary` 전에 차단되는지 검증한다.
- menu query 오류는 `admissionFailed`와 fail-closed session error로 분류돼야 한다.
- report test는 `blockedLocalMenu`가 boundary conservation에 포함되지만 stage failure와 not-ready drop은
  아닌지 검증한다.
- failure priority test는 heartbeat가 0이어도 유효한 capture failure evidence가 있으면
  `control_flow_failure_observed`가 대표 code인지 검증한다.
- 실제 S-B/S-A 실행은 menu 없음 왕복과 menu-open 차단을 같은 clean v16 build로 각각 수행한다. menu-open
  실행에서 S-B `blockedLocalMenu>0`, request write 0, S-A entry 0이어야 한다.
