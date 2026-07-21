# ADR 0029: source focus 복원에도 bounded thread-input fallback을 사용한다

- 상태: Experimental
- 책임자: cherr
- 범위: Windows source capture deactivation, focus 복원과 Phase 0 evidence
- 관계: ADR 0028의 foreground activation fallback을 source focus 복원에 대칭 적용한다. ADR 0027의 two-phase entry와 local interaction admission은 유지한다.

ADR 0030은 v19 실기 결과를 근거로 정확한 focus 복원 실패의 terminal 정책을 좁힌다. bounded fallback 자체와
detach failure의 terminal 정책은 유지된다.

## 맥락

2026-07-18 gram13의 `agent-session/v18` r3 실기 실행은 boundary intent `12`건 중 `10`건을 승인했고,
capture activation과 commit을 `10/10` 완료했다. 첫 foreground activation이 거절된 `9`건도 ADR 0028
fallback으로 모두 성공했다. HP15는 entry와 return arm을 `10/10`, revoke를 `10`건 남겼다.

하지만 capture deactivation에서 gram13의 저장된 foreground application을 복원한 결과는 성공 `1`, 실패
`9`였고 실패는 모두 `focusRestoreApiRejected`였다. 각 실패가 source hook을 종료해 gram13은
`source-hook/runtime/component_failed`와 local connection interrupt를 각각 `9`건, HP15는
`peer-reader/read/remote_eof`를 `9`건 기록했다. 재연결 공백 동안 추가 boundary intent `2`건이
`droppedNotReady`가 되어 사용자가 간헐적인 왕복 실패를 체감했다. 최대 receive gap은 `522ms`로
`3s` timeout보다 작았고 entry, injection과 guard failure는 없었으므로 network나 destination 입력 실패가
아니라 source focus 복원이 첫 divergence다.

복원 실패를 무시하면 capture window를 숨긴 뒤 keyboard focus owner를 확인할 수 없고 실제 focus 결함을
성공으로 가장한다. 반대로 모든 API 거절을 terminal로 유지하면 이미 성공한 entry fallback 뒤마다 연결을
재생성하게 된다.

## 결정

- deactivation은 touchpad registration 해제 뒤 capture window가 여전히 foreground이고 저장한 focus HWND가
  유효할 때만 복원을 시도한다. 이 조건이 아니면 기존처럼 skipped로 분류한다.
- 저장한 focus HWND에 대한 기존 `SetForegroundWindow`와 즉시 검증을 항상 첫 경로로 사용한다.
- 첫 호출이 API에서 거절되거나 호출 직후 저장한 HWND가 foreground로 확인되지 않은 경우에만 focus restore
  fallback을 한 번 시도한다.
- fallback은 capture window를 소유한 고정 OS thread와 저장한 focus HWND의 GUI thread를
  `AttachThreadInput(TRUE)`로 잠시 결합한다. 같은 thread, 사라진 HWND, thread 조회 실패와 attach 실패는
  fail-closed한다.
- 결합 뒤 `SetForegroundWindow(savedFocus)`를 정확히 한 번 재시도하고 실제 foreground owner를 확인한다.
  성공·실패와 무관하게 `AttachThreadInput(FALSE)`를 반드시 호출한다. retry loop는 없다.
- fallback까지 성공하면 최종 focus 결과는 restored이며 source session을 실패시키지 않는다.
- attach, detach, 재시도 API 또는 verification 실패는 기존처럼 terminal source-capture failure다. capture
  window는 숨기고 capture thread 종료로 남은 queue attachment를 폐기한다.
- HWND, thread ID, process, window title과 input 값은 report에 저장하지 않는다.

## evidence 계약

`krasis.lab.agent-session/v19`의 `sourcePipeline.wheel`은 다음 counter를 추가한다.

- `focusRestoreFallbackAttempted`
- `focusRestoreFallbackSucceeded`
- `focusRestoreFallbackAttachFailed`
- `focusRestoreFallbackDetachFailed`
- `focusRestoreFallbackApiRejected`
- `focusRestoreFallbackVerificationLost`

다섯 outcome의 합은 attempted와 정확히 같아야 한다. fallback success는 최종 `focusRestored`보다 클 수 없고,
fallback failure 합은 최종 `focusRestoreFailed`보다 클 수 없다. fallback 성공은 최초 일반 API 거절을 최종
focus failure로 세지 않는다.

terminal failure triple은 `source-capture/focus-restore` phase 아래 `api_rejected`,
`verification_lost`, `thread_attach_failed`, `thread_detach_failed`로 분류한다. strict pair checker는
`krasis.lab.agent-session-check/v16`, Phase 0 aggregate는 `krasis.lab.phase0-check/v16`이다.

## 결과

정상 focus 복원에는 새 API를 사용하지 않는다. Windows가 정상 복원을 거절한 실행만 짧은 queue attachment를
사용하며, 최종 저장 HWND가 실제 foreground로 확인된 경우에만 세션을 계속한다. 실패를 숨기지 않고
원인별 aggregate와 terminal failure를 남긴다.

hosted test는 일반 경로 우선, 단일 retry, 항상 detach, 성공 시 non-terminal 결과와 실패 분류만 증명한다.
gram13에서 r3의 `1/10` focus 복원이 개선되는지, menu·drag와 keyboard focus에 부작용이 없는지는 v19 실제
장비 검증이 필요하다.

## 대안

- focus restore 실패를 skipped로 강등: 실제 keyboard focus owner를 확인하지 않고 성공으로 가장해 기각한다.
- capture 활성화 때 만든 thread attachment를 session 동안 유지: input queue 결합 범위가 너무 커 기각한다.
- `SetForegroundWindow` 무한 retry 또는 delay 증가: 사용자 focus를 반복 탈취하고 원인을 숨겨 기각한다.
- capture window를 먼저 숨기고 Windows 자동 focus 선택에 맡김: 저장 application 복원을 보장하지 않아 기각한다.

## 검증 gate

- 정상 복원은 fallback counter를 증가시키지 않아야 한다.
- 일반 복원 거절 뒤 fallback 성공은 attempted/succeeded `1/1`, 최종 restored `1`을 남기고 source hook과
  connection을 종료하지 않아야 한다.
- attach, detach, retry API와 verification 실패는 정확한 outcome과 failure triple을 남기고 capture window를
  숨긴 뒤 안전 종료해야 한다.
- gram13/HP15에서 menu와 drag가 없는 왕복을 최소 `15`회 수행하며 `droppedNotReady=0`, entry/return arm과
  commit/revoke가 모두 일치해야 한다.
- gram13 menu, mouse capture와 pressed input은 계속 request 전에 차단되고, 닫은 뒤 rearm하면 다시 전환돼야 한다.
- HP15에서 pointer, click, drag, keyboard, vertical/horizontal touchpad scroll이 동작하고 gram13 local
  application에 wheel이 남지 않아야 한다.
- 강제 종료와 peer disconnect 뒤 thread attachment, capture window, registration과 suppression이 남지 않아야 한다.

## 참고

- [AttachThreadInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-attachthreadinput)
- [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow)
