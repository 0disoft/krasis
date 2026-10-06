# ADR 0023: active outbound에서 touchpad-capable window가 wheel을 소유한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 Windows source wheel 전달과 foreground lifecycle
- 관계: ADR 0013의 상시 hook-owned wheel을 active outbound 구간에 한해 대체하고, ADR 0022의 진단 결과를 production-shaped lab 경로로 승격한다. foreground 거절 fallback은 ADR 0028이 소유한다.

## 맥락

gram13의 `touchpad-surface` 결과는 foreground touchpad-capable window가 `PT_TOUCHPAD` pointer
message `1850`건을 받고, 이를 `DefWindowProc`에 넘긴 같은 실행에서 legacy vertical wheel `396`건과
horizontal wheel `297`건을 받았음을 보였다. pointer type 조회 실패와 foreground 이탈은 `0`이었다.

반면 이전 `agent-session/v12` active outbound에서는 pointer move와 button callback이 살아 있었지만
`WH_MOUSE_LL`, synthesized hook과 Raw Input wheel이 모두 `0`이었고 두 손가락 scroll은 source local
window에 남았다. 따라서 queue나 network를 다시 바꾸는 것으로는 touchpad wheel을 얻을 수 없다.

Windows 11은 touchpad-capable window가 두 손가락 gesture의 `WM_POINTER`를 받으며, 이를
`DefWindowProc`에 넘기면 system legacy mouse wheel message로 변환한다고 명시한다. 직접 gesture를
해석하지 않고 이 변환을 이용하면 Interaction Context를 복제하지 않고 현재 system 변환 결과를 사용할
수 있다.

## 결정

- source hook은 전용 OS thread와 숨은 top-level tool window를 추가한다. window는 source hook
  session마다 하나만 존재하고 평소에는 숨긴다.
- exact outbound lease grant 뒤 `SourceSession`을 먼저 활성화하고 capture window를 활성화한다.
  window activation이 실패하면 source session을 즉시 비활성화하고 runtime grant를 fault cleanup한다.
- capture activation은 현재 cursor를 덮는 `24x24` topmost tool window를 배치하고
  `RegisterTouchpadCapableWindow(TRUE)` 뒤 show와 foreground 전환을 수행한다. foreground 전환 결과를
  실제 `GetForegroundWindow`로 재확인한다.
- active source path는 user32의 named `RegisterTouchpadCapableWindow` export만 허용한다. 이름이 없으면
  unsupported로 실패하며 ordinal fallback을 호출하지 않는다. lab-only `touchpad-surface`의 ordinal
  fallback은 API surface 진단에만 남긴다.
- active window의 touchpad `WM_POINTER`는 항상 `DefWindowProc`에 넘긴다. 결과로 받은
  `WM_MOUSEWHEEL`과 `WM_MOUSEHWHEEL`의 signed delta만 normalized wheel action으로 만들어 source
  queue에 non-blocking offer한다.
- active capture 동안 physical 또는 untagged synthesized low-level hook wheel은 관찰만 하고
  hook chain으로 통과시킨다. capture window가 최종 wheel mapper이므로 외장 mouse도 같은 message를
  두 번 enqueue하지 않는다.
- Krasis/self-test tag wheel은 low-level hook에서 삼켜 capture window와 원격 queue로 다시 들어가지
  못하게 한다.
- local return, peer revoke와 session failure는 capture window를 먼저 비활성화한 뒤 source session을
  비활성화한다. 이 순서로 window가 살아 있는데 queue만 닫힌 구간을 만들지 않는다.
- 비활성화는 touchpad registration 해제, 이전 foreground window의 조건부 복원, window hide 순서다.
  capture window를 먼저 숨기면 Windows가 다른 foreground owner를 고른 뒤라 저장한 focus를 정상적으로
  복구할 수 없으므로, capture가 아직 foreground일 때만 복원을 시도하고 성공·실패와 무관하게 숨긴다.
  capture window가 더 이상 foreground가 아니거나 이전 handle이 유효하지 않으면 복원을 건너뛴다.
  실제 복원 시도 실패는 source hook failure로 승격한다.
- active capture가 `WM_ACTIVATE/WA_INACTIVE`를 받으면 즉시 source suppression을 해제하고
  queued input generation을 폐기한다. agent는 write coordinator에서 현재 outbound lease를
  해제하고 기존 revoke frame으로 peer에 통지한다. cleanup이 끝나기 전에는 새 outbound
  activation을 막고, 정상 해제 뒤에는 owner thread와 인증된 연결을 재사용한다.
  native cleanup·peer 통지 실패는 기존 terminal failure 경로를 따른다.
- source hook cancellation 중 unregister, focus restore 또는 window cleanup 오류는 상위 cancellation에
  가려지지 않고 함께 반환한다.

## evidence 계약

현재 `krasis.lab.agent-session/v21`의 `sourcePipeline.wheel`은 v14에서 확정한 다음 counter를 유지하고,
ADR 0028의 bounded foreground activation fallback, ADR 0029의 focus restore fallback과 ADR 0030의
post-hide focus release outcome을 추가한다.

- `windowObserved`: active capture window에서 받은 vertical/horizontal legacy wheel 수
- `captureActivations`, `captureDeactivations`: 성공한 capture lifecycle 수
- `focusRestoreSkipped`, `focusRestored`, `focusRestoreFailed`: 성공한 activation의 deactivation별 focus 결과.
- `focusRestoreApiRejected`, `focusRestoreVerificationLost`: 실패 합계를 각각 `SetForegroundWindow` 거절과
  호출 성공 직후 저장한 foreground owner가 확인되지 않은 경합으로 나눈 원인별 counter.
  foreground 획득 전에 실패한 activation의 cleanup은 이 lifecycle counter에 섞지 않는다.
- `foregroundFallbackAttempted`와 성공·attach 실패·detach 실패·재시도 API 거절·재검증 상실 counter:
  정상 foreground 경로가 실패했을 때만 수행한 ADR 0028 fallback의 단일 outcome.
- `focusRestoreFallbackAttempted`와 같은 다섯 outcome: 정상 saved-focus 복원이 실패했을 때만 수행한
  ADR 0029 fallback의 단일 outcome.
- `postHideFocusAttempted`와 recovered·unavailable·capture-retained outcome: 정확한 restore failure 뒤
  capture window를 숨긴 결과. ADR 0030에 따라 앞의 두 결과는 degraded, capture-retained는 unsafe다.

`mapped`는 hook physical/synthesized와 window observation 합계보다 클 수 없다. clean report는 capture
activation/deactivation 수가 같고, 모든 deactivation의 focus 결과가 정확히 하나로 분류돼야 한다.
`focusRestoreApiRejected+focusRestoreVerificationLost=focusRestoreFailed`이고 restore failure는 post-hide outcome으로
모두 보존돼야 한다. capture-retained 또는 thread detach failure가 없어야 한다. `windowObserved>0 && sent=0`은
`window_wheel_observed_without_forwarding`으로 실패한다.

strict pair checker는 `krasis.lab.agent-session-check/v18`, Phase 0 aggregate는
`krasis.lab.phase0-check/v17`이다. v15가 추가한 boundary handoff와 failure origin evidence는 ADR 0024,
v16 local menu admission은 ADR 0025가 소유하며 wheel owner와 focus 복원 정책은 바꾸지 않는다.

## 결과

touchpad와 외장 mouse wheel은 active outbound에서 같은 window-owned queue ingress를 사용한다.
low-level hook은 외장 mouse wheel을 보더라도 window owner가 active면 payload를 만들지 않는다.

2026-07-16 S-B source와 S-A destination의 실제 `agent-session/v13`에서 destination scroll이 동작했다.
source의 `windowObserved`, `queueAccepted`, `sent`와 destination `wheels`가 모두 `601`로 이어졌고 capture
activation/deactivation은 `7/7`, focus restore failure와 injection failure는 `0`이었다. 양쪽 crash
guard도 clean disarm과 remaining `0`을 남겼다. redacted 실행 기록은
`docs/experiments/results/2026-07-16-study-pair-v13.md`에 있다.

2026-07-17 같은 pair의 clean commit artifact로 12회 왕복한 실행은 destination entry/return arm `12/12`,
source/destination wheel `57/57`과 안전한 guard 종료를 남겼지만 focus 복원 한 번이 실패했다. v13은 이
한 건을 원인 없는 `focusRestoreFailed`로만 남겼다. v14는 안전 정책을 완화하지 않고 API 거절과 호출 후
verification 경합을 분리해 다음 실제 장비 실행이 정책 판단에 필요한 원인을 남기게 한다.

capture window는 foreground를 가져가므로 Phase 0에서 눈에 보이는 작은 edge surface가 생길 수 있다.
원격 제어 중 source의 일반 application focus를 유지하는 제품 UX는 아직 확정하지 않는다. Wails 설정 UI,
tray, installer와 자동 시작은 이 ADR 범위 밖이다.

### Fullscreen taskbar presentation

When the previous foreground window's client surface covers its monitor, mark
the capture window with `ITaskbarList2::MarkFullscreenWindow(TRUE)` before showing
or activating it. This preserves Shell fullscreen presentation while retaining
the verified touchpad wheel owner. A maximized work-area client is not fullscreen.
Inspect only client and monitor geometry; do not read window titles or browser content.

After restoring focus and hiding capture, clear its marking. Activation rollback
and repeated deactivation also clear it. Only the Krasis capture HWND is marked;
the browser HWND, Explorer taskbar windows, and global taskbar settings are not modified.
Shell API failure degrades presentation without interrupting input or weakening
foreground verification. Native hidden-window and lifecycle tests cover API
acceptance, ordering, normal windows, scaled/negative monitor coordinates, and rollback.
The reported YouTube fullscreen A-to-B behavior still requires a two-PC check.

- [MarkFullscreenWindow](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-itaskbarlist2-markfullscreenwindow)

## 대안

- Raw pointer update로 직접 pan delta 계산: system gesture 설정과 inertia를 다시 구현해야 하므로 기각한다.
- Interaction Context로 직접 gesture 인식: 장기 제품 후보지만 현재 확인된 `DefWindowProc` legacy 변환보다
  구현·검증 범위가 크므로 보류한다.
- low-level hook과 window가 모두 wheel payload 생성: 외장 mouse 한 notch가 두 번 전달될 수 있어
  기각한다.
- foreground 전환 실패를 무시하고 session 유지: touchpad scroll이 local application에 남는 기존 결함을
  성공으로 가장하므로 기각한다.
- `AttachThreadInput`으로 강제 foreground 탈취: thread input queue 결합과 복원 실패 위험이 커 Phase 0에
  일반 경로와 session 전체 결합에는 도입하지 않는다. 2026-07-18 v17에서 정상 interaction 중에도
  foreground API 거절이 `6/8` 발생한 뒤 ADR 0028이 단일 retry와 즉시 detach로 범위를 제한한 fallback만
  실험적으로 허용했다.

## 검증 gate

- gram13 touchpad vertical/horizontal scroll이 HP15에서 각각 동작하고 gram13 local window에는 남지 않는다.
- gram13에 연결한 일반 mouse wheel 한 notch가 HP15에서 한 번만 적용된다.
- source report에서 `captureActivations>=1`, `captureActivations=captureDeactivations`, focus 결과 합계가
  deactivation과 같아야 한다. restore failure가 있으면 post-hide outcome으로 모두 분류되고
  capture-retained와 thread detach failure가 없어야 한다.
- touchpad 또는 mouse wheel 구간에서 `windowObserved`, `mapped`, `queueAccepted`, `sent`가 모두 양수이고
  destination `wheels`도 양수여야 한다.
- return 뒤 이전 source focus가 복원·skipped되거나 post-hide release로 degraded 분류되고 capture surface가 사라져야 한다.
- active 중 다른 창으로 전환하면 session이 source-hook failure로 종료되고 양쪽이 `LOCAL`로 수렴해야 한다.
- 강제 종료와 peer disconnect 뒤 capture window, touchpad registration과 suppression이 남지 않아야 한다.

첫 번째, 세 번째와 네 번째 gate의 기본 touchpad 전달 부분은 2026-07-16 S-A/S-B 실행에서 통과했다.
가로 scroll 분리, 외장 mouse 단일 적용과 마지막 세 장애 gate는 아직 열려 있다.

## 참고

- [RegisterTouchpadCapableWindow](https://learn.microsoft.com/en-us/windows/win32/input-precisiontouchpad/registertouchpadcapable)
- [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow)
