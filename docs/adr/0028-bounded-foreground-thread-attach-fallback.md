# ADR 0028: foreground 거절 때만 bounded thread-input fallback을 사용한다

- 상태: Experimental
- 책임자: cherr
- 범위: Windows source capture foreground activation과 Phase 0 evidence
- 관계: ADR 0023의 일반 activation 경로를 유지하되 `AttachThreadInput` 전면 기각을 좁은 fallback에 한해 대체한다. ADR 0027의 two-phase entry와 abort rollback은 그대로 유지한다. deactivation focus 복원 fallback은 ADR 0029가 소유한다.

## 맥락

2026-07-18 gram13의 `agent-session/v17` 실기 실행은 boundary request, grant apply와 source session
activation을 `8/8` 완료했지만 capture activation은 `2/8`만 성공했다. 나머지 `6`건은 모두
`captureActivationApiRejected`였고 setup failure와 verification loss는 `0`이었다. source는 성공 두 건만
`LeaseCommit`하고 실패 여섯 건은 `LeaseAbort`했다. HP15는 commit `2`, abort와 entry rollback `6/6`,
rollback restored `6/6`을 남겼다. 따라서 two-phase entry는 half-entry를 막았지만 Windows의
foreground 정책 때문에 정상적인 반복 전환이 지나치게 자주 거절됐다.

`SetForegroundWindow`를 무조건 반복하거나 사용자 입력을 강제로 풀면 foreground 탈취와 local interaction
파괴 위험이 생긴다. 반대로 첫 거절을 그대로 종료하면 gram13에서 실사용 가능한 왕복 비율이 낮다.
Windows의 `AttachThreadInput`은 두 GUI thread의 input queue를 잠시 결합해 foreground 전환을 허용할 수 있지만,
결합을 남기거나 반복하면 focus, capture와 keyboard state를 함께 오염시킬 수 있다.

## 결정

- 기존 `SetForegroundWindow`와 즉시 `GetForegroundWindow` 검증을 항상 첫 경로로 사용한다. 정상 경로가
  성공하면 thread input queue를 결합하지 않는다.
- 첫 호출이 API에서 거절되거나 호출 직후 capture window 소유권이 확인되지 않은 경우에만 fallback을 한 번
  시도한다.
- fallback은 capture window를 소유한 고정 OS thread와 그 순간의 foreground window thread를 식별한다.
  handle 또는 thread ID가 없거나 두 thread가 같으면 fail-closed한다.
- `AttachThreadInput(TRUE)`가 성공한 경우 `SetForegroundWindow`를 정확히 한 번 재시도하고 즉시 실제
  foreground owner를 검증한다. 성공·실패와 무관하게 `AttachThreadInput(FALSE)`를 반드시 호출한다.
- attach, detach, 재시도 API 또는 재검증이 하나라도 실패하면 capture activation을 성공으로 만들지 않는다.
  source는 `LeaseAbort`를 쓰고 destination은 prepared entry와 cursor를 rollback한다. retry loop는 없다.
- detach 실패는 terminal source-capture failure다. capture thread를 종료해 Windows가 thread 종료와 함께
  남은 queue attachment를 폐기하게 하고, 같은 process에서 재사용하지 않는다.
- report에는 HWND, thread ID, window title 또는 process 정보를 저장하지 않는다. aggregate outcome만 남긴다.

## evidence 계약

`krasis.lab.agent-session/v18`의 `sourcePipeline.wheel`은 다음 counter를 추가한다.

- `foregroundFallbackAttempted`
- `foregroundFallbackSucceeded`
- `foregroundFallbackAttachFailed`
- `foregroundFallbackDetachFailed`
- `foregroundFallbackApiRejected`
- `foregroundFallbackVerificationLost`

다섯 outcome의 합은 attempted와 정확히 같아야 한다. fallback success는 성공한 capture activation보다 클 수
없고 fallback failure 합은 capture activation failure보다 클 수 없다. supervisor failure triple은 attach와
detach 실패를 각각 `source-capture/capture-activate/thread_attach_failed`와
`source-capture/capture-activate/thread_detach_failed`로 분류한다. 기존 setup, API rejection과 verification
loss 분류도 raw source-hook failure로 뭉개지지 않는다.

strict pair checker는 `krasis.lab.agent-session-check/v15`, Phase 0 aggregate는
`krasis.lab.phase0-check/v15`다.

## 결과

정상 foreground 전환에는 새 위험을 추가하지 않는다. 거절된 실행만 짧은 queue attachment를 사용하며,
성공을 확인한 경우에만 two-phase commit이 진행된다. 실패는 원격 cursor를 먼저 넘기지 않고 원인별 evidence를
남긴다.

이 결정은 실제 gram13/HP15 desktop에서 아직 검증되지 않았다. hosted test는 API 호출 순서, 단일 retry,
항상 detach, 실패 rollback과 counter 보존만 증명한다. Windows foreground 정책이 실기에서 fallback을
허용하는지와 focus/capture 부작용이 없는지는 새 v18 artifact로 확인해야 한다.

## 대안

- 첫 실패를 그대로 종료: 안전하지만 v17에서 8회 중 6회가 거절돼 반복 사용성이 부족했다.
- `SetForegroundWindow`만 여러 번 반복: 사용자 foreground를 반복 탈취하고 성공 조건도 명확하지 않아 기각한다.
- `AllowSetForegroundWindow` 또는 별도 foreground broker: 상대 process 협조나 추가 권한 경계가 필요해 현재
  Phase 0보다 범위가 크므로 보류한다.
- local application의 capture나 menu를 강제 해제: 사용자의 drag와 상태를 파괴하므로 계속 금지한다.
- attach 상태를 session 동안 유지: 두 thread input queue의 결합 범위가 너무 커 기각한다.

## 검증 gate

- menu, drag, pressed input과 move/size는 기존 local admission에서 계속 request 전에 차단돼야 한다.
- 정상 activation은 fallback attempted를 증가시키지 않아야 한다.
- 첫 foreground 거절 뒤 fallback 성공은 attempted/succeeded `1/1`, capture activation과 commit `1`을 남겨야 한다.
- attach, detach, 재시도 API와 verification 실패는 각각 정확한 aggregate와 failure triple을 남기고 abort와
  destination rollback으로 끝나야 한다.
- gram13에서 menu와 drag가 없는 반복 왕복이 v17의 `2/8`보다 안정적으로 완료돼야 한다.
- 왕복 뒤 gram13의 원래 application focus가 복원되거나 안전하게 skipped로 분류돼야 하며 capture window가
  남지 않아야 한다.
- touchpad vertical/horizontal wheel, click, drag와 keyboard는 HP15에서 동작하고 gram13 local application에
  wheel이 남지 않아야 한다.
- 강제 종료와 peer disconnect 뒤 input queue attachment, capture window, touchpad registration과 suppression이
  남지 않아야 한다.

## 참고

- [AttachThreadInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-attachthreadinput)
- [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow)
