# 2026-07-17 S-A/S-B clean v13 다중 왕복 검증

- 판정: `PARTIAL` — 입력과 return은 통과, source focus 복원 1회 실패
- 전체 Phase 0 판정: `IN_PROGRESS`
- 장소: 스터디카페
- source: `S-B`, `connect`, `LEFT`
- destination: `S-A`, `listen`, `RIGHT`
- agent schema: `krasis.lab.agent-session/v13`
- crash guard schema: `krasis.lab.crash-guard/v1`
- build revision: `1a25f16b75ff3d6b5c76ca905e36810bdf7ebe24`
- build modified: `false`
- CI artifact ZIP SHA-256: `10852af643bf9ecccfcccec41e7f641cb6b5a52d8e74f9a4e65088bb9db9f233`

## 목적과 실행

clean commit의 CI artifact로 S-B에서 S-A로 진입하고 다시 S-B로 돌아오는 동작을 반복해 touchpad wheel,
pointer, button, keyboard, return hysteresis와 capture focus lifecycle을 함께 확인했다. 실행 시간은 `45s`,
heartbeat interval은 `500ms`, peer timeout은 `3000ms`였다. 사용자는 처음 한 번 return이 안 된 것으로
판단했으나 32px hysteresis를 지난 뒤 복귀가 정상 동작함을 확인했고, 보고서를 지운 뒤 12회 왕복 실행을
새로 수집했다.

## redacted 결과

| 항목 | S-B source | S-A destination | 판정 |
| --- | ---: | ---: | --- |
| entry/return | boundary intent `12` | entry/return arm `12/12` | 통과 |
| pointer move | `1080` | `1003` | 동작 확인 |
| key down/up | `2/2` | `2/2` | 일치 |
| button down/up | `9/9` | `9/9` | 일치 |
| wheel | sent `57` | injected `57` | 일치 |
| capture lifecycle | activation/deactivation `12/12` | 해당 없음 | 균형 |
| focus restore | restored `11`, failed `1` | 해당 없음 | 실패 |
| injection failure | 해당 없음 | 네 분류 모두 `0` | 통과 |
| crash guard | remaining `0`, passed | remaining `0`, passed | 통과 |

source의 hook observation `148` 중 active queue가 받은 wheel은 `57`이고 나머지 `91`은 return 뒤 inactive
구간으로 분류됐다. `queueAccepted=sent=57`이며 destination wheel도 `57`이므로 이 실행에서 wheel 전달
손실 증거는 없다. destination은 entry와 return arm을 각각 `12`회 기록했으므로 왕복 경계 경로도
작동했다.

S-B report가 실패한 직접 원인은 `invalid_source_pipeline_evidence`다. capture activation/deactivation은
`12/12`로 닫혔지만 focus 결과가 restored `11`, failed `1`이었다. source hook component가 한 번
재시작됐고 마지막 failure component는 `source-hook`이었다. v13은 실패 원인을 더 나누지 않으므로
`SetForegroundWindow` 거절인지, 호출 성공 직후 다른 foreground owner가 이긴 경합인지 이 기록만으로는
판단할 수 없다.

## 후속 결정

실패를 재실행으로 지우거나 경고로 낮추지 않는다. `agent-session/v14`는 기존 `focusRestoreFailed` 합계를
유지하면서 `focusRestoreApiRejected`와 `focusRestoreVerificationLost`를 추가한다. 두 원인별 counter의
합이 실패 합계와 맞지 않으면 strict checker가 report를 거부한다. 실제 복원 실패는 원인이 확인될 때까지
계속 source hook failure와 session fail-closed 종료를 일으킨다.

다음 장비 실행은 v14 clean artifact로 여러 번 왕복해 두 원인 중 어느 counter가 증가하는지 확인한다.
그 뒤 verification 경합만 제품 UX 저하로 취급할 수 있는지, 아니면 현재처럼 session-terminal로 유지할지
별도 ADR 판단을 내린다.
