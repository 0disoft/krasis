# ADR 0030: 안전하게 foreground를 놓았다면 정확한 focus 복원 실패로 세션을 종료하지 않는다

- 상태: Experimental
- 책임자: cherr
- 범위: Windows source capture deactivation, post-hide focus 검증과 Phase 0 evidence
- 관계: ADR 0029의 bounded focus restore fallback은 유지하되 terminal 정책을 좁힌다. ADR 0027의 two-phase entry와 local interaction admission은 변경하지 않는다.

## 맥락

2026-07-18 gram13의 `agent-session/v19` 실기 실행은 boundary intent `23`건 중 `19`건을 승인했고,
request, grant, source activation, capture activation, commit과 revoke를 승인된 `19/19` 모두 완료했다. HP15도
entry, return arm과 revoke를 `19/19` 완료했고 pointer `2615`, wheel `284`를 injection failure 없이 적용했다.
capture 진입 foreground fallback도 gram13에서 `8/8` 성공했다.

남은 결함은 return deactivation이었다. gram13의 정확한 saved-focus 복원은 `19`회 중 `14`회 성공하고 `5`회
실패했다. bounded focus fallback은 `8`회 중 `3`회만 성공했으며 나머지 `5`회는 API 거절이었다. 이 중 네
번은 supervisor의 `source-capture/focus-restore` failure로 귀속됐고 연결 재생성 동안 boundary intent `4`건이
`droppedNotReady`가 됐다. 사용자가 왼쪽 경계를 여러 번 밀어야 했던 체감과 일치한다. 양쪽 최대 receive
gap은 각각 `1249ms`, `1352ms`로 `3s` timeout보다 작아 network timeout이 첫 divergence가 아니다.

`SetForegroundWindow(savedFocus)`는 이전 application을 곧바로 다시 사용할 수 있게 하는 편의 동작이다.
반면 remote input을 끝내고 capture window를 숨겨 local input 소유권을 놓는 것은 안전 경계다. 두 동작을
같은 terminal 조건으로 묶으면 Windows가 정확한 application focus 복원만 거절해도 정상 peer session과
source hook까지 반복해서 재생성된다.

## 결정

- 정상 `SetForegroundWindow(savedFocus)`와 ADR 0029의 bounded attach/retry/detach fallback은 그대로 먼저
  실행한다. 정확한 saved-focus 복원 성공이 최선의 경로다.
- 정확한 복원이 실패해도 touchpad registration을 해제하고 capture window를 반드시 숨긴다.
- 숨긴 직후 foreground owner를 aggregate-only로 확인한다.
  - non-zero이며 capture window가 아니면 `recovered`다.
  - foreground owner가 없으면 `unavailable`이다. capture window가 소유권을 놓았으므로 local click으로
    다시 선택할 수 있는 degraded 상태다.
  - 숨긴 capture window가 계속 foreground면 `capture_retained`이며 terminal failure다.
- `recovered`와 `unavailable`은 정확한 focus 실패를 report에 남기되 source hook, peer connection과
  supervisor session을 종료하지 않는다.
- `AttachThreadInput(FALSE)` 실패는 post-hide 결과와 무관하게 terminal이다. input queue attachment가 남을
  수 있기 때문이다. registration 해제 실패와 기존 activation 안전 실패도 terminal 정책을 유지한다.
- hidden capture retention terminal failure는 `source-capture/focus-release/capture_retained`로 분류한다.
- HWND, thread ID, process ID, window title과 입력 내용은 report에 저장하지 않는다.

## evidence 계약

`krasis.lab.agent-session/v20`의 `sourcePipeline.wheel`은 다음 counter를 추가한다.

- `postHideFocusAttempted`
- `postHideFocusRecovered`
- `postHideFocusUnavailable`
- `postHideFocusCaptureRetained`

세 outcome의 합은 attempted와 같아야 하고 attempted는 `focusRestoreFailed`와 같아야 한다. 정확한 focus
복원이 실패하지 않았다면 post-hide counter는 모두 `0`이다. `postHideFocusCaptureRetained>0` 또는
`focusRestoreFallbackDetachFailed>0`은 unsafe lifecycle이다. 그 외 restore failure가 recovered 또는
unavailable로 완전히 분류되면 source pipeline은 구조적으로 유효하다. focus fallback의 네 failure outcome
합도 최종 `focusRestoreFailed`와 정확히 같아야 한다.

strict pair checker는 `krasis.lab.agent-session-check/v17`, Phase 0 aggregate는
`krasis.lab.phase0-check/v17`이다.

## 결과

Windows가 saved application focus를 돌려주지 않아도 capture window가 foreground를 놓았다면 연결을
재생성하지 않는다. 사용자는 정확한 이전 application 대신 다른 local window나 foreground가 없는 상태로
돌아올 수 있으므로 사용성은 degraded일 수 있지만, 다음 boundary intent가 reconnect 공백 때문에 버려지는
연쇄 실패는 막는다.

hosted test는 정확한 restore 우선, degraded release의 non-terminal 처리, capture retention과 detach failure의
terminal 처리, counter 합계만 증명한다. gram13에서 connection interrupt와 `droppedNotReady`가 실제로
사라지는지는 v20 두 장비 검증이 필요하다.

## 대안

- 모든 focus 복원 실패를 계속 terminal로 처리: v19에서 재연결 공백과 실제 왕복 실패를 만들었으므로 기각한다.
- 모든 restore 오류를 기록 없이 무시: hidden capture retention과 detach failure까지 숨기므로 기각한다.
- `SetForegroundWindow`를 성공할 때까지 반복: Windows focus 정책을 우회하려는 retry loop가 되므로 기각한다.
- saved application에 synthetic input을 보내 foreground 권한을 얻음: 사용자 입력을 위조하고 대상 application에
  부작용을 만들 수 있어 기각한다.

## 검증 gate

- 정상 restore와 ADR 0029 fallback 성공은 기존처럼 `focusRestored`를 남기고 post-hide counter는 증가하지 않아야 한다.
- restore API 또는 verification 실패 뒤 capture release가 확인되면 failure counter와 post-hide outcome을 모두
  남기되 source hook과 peer connection을 종료하지 않아야 한다.
- hidden capture retention과 thread detach 실패는 정확한 terminal failure evidence를 남겨야 한다.
- gram13/HP15에서 menu와 drag가 없는 왕복을 최소 `15`회 수행하며 `droppedNotReady=0`, focus restore 실패로
  인한 local connection interrupt 증가가 없어야 한다.
- 정확한 focus가 degraded된 회차에는 gram13 keyboard 입력 대상이 안전한 local window인지 수동 확인한다.
- HP15의 pointer, click, drag, keyboard와 vertical/horizontal touchpad scroll 회귀가 없어야 한다.

## g170 restore refinement (retired in g171)

g169 device evidence shows four rejected attached restore calls where capture
remains foreground before and after detach, followed by NULL after hide. Five
subsequent activations start with NULL and fail. The degraded-release policy remains,
but one restoration-only queue-local activation is added inside the existing
successful attachment to reduce this failure chain.

After the attached SetForegroundWindow(savedFocus) call rejects, SetActiveWindow
is requested once only when the saved target is still valid, visible, enabled and
responsive to the existing probe; the exact target/peer is savedFocus; the run is
not terminal; and capture still owns foreground immediately before the request.
NULL, an external foreground, hidden/disabled/destroyed targets and capture
activation never use this path. No extra attachment cycle or synthesized input is
introduced. Diagnostics are optional and never gate this runtime operation.

The API return is not ownership proof. The saved target must be observed as actual
foreground while joined, and the existing ownership check after detach must also
pass. A lost post-detach observation uses only the existing bounded completion
probe; it does not repeat activation. Detach failure remains terminal regardless
of the earlier observation. If local activation is unverified, preserve the
original API rejection and degraded-hide policy. The existing native-call
responsiveness risk of joined queues remains; a finite call count does not prove
a wall-clock latency bound. Device verification is required before claiming the
click-recovery symptom is resolved.

g170 device observations exercised this request eleven times without observing
the saved target as foreground. g171 removes the cross-thread SetActiveWindow
request and its eligibility probes. Microsoft documents that a target created by
another thread sets the calling thread's active status to NULL. The existing
SetForegroundWindow/attach/detach path and degraded-hide policy remain. Historical
restoreActive fields remain readable, but g171 does not emit them.

## 참고

- [SetActiveWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setactivewindow)
- [Input queue synchronization](https://devblogs.microsoft.com/oldnewthing/20130607-00/?p=4143)
- [SetForegroundWindow restrictions](https://learn.microsoft.com/en-us/windows/win32/winmsg/window-features)
- [AttachThreadInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-attachthreadinput)
