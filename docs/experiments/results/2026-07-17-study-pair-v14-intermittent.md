# 2026-07-17 S-A/S-B v14 간헐적 왕복 관찰

- 판정: `PARTIAL` — 입력 전달은 통과, 일부 경계 전환은 간헐적으로 지연 또는 누락
- 전체 Phase 0 판정: `IN_PROGRESS`
- 장소: 스터디카페
- source: `S-B`, `connect`, `LEFT`
- destination: `S-A`, `listen`, `RIGHT`
- agent schema: `krasis.lab.agent-session/v14`
- crash guard schema: `krasis.lab.crash-guard/v1`
- build revision: `1a25f16b75ff3d6b5c76ca905e36810bdf7ebe24`
- build modified: `true`

## 목적과 실행

v14 focus 복원 원인 계측 build로 S-B에서 S-A로 진입하고 복귀하는 동작을 `90s` 동안 반복했다. heartbeat
interval은 `500ms`, peer timeout은 `3000ms`였다. pointer, button과 touchpad wheel 전달은 동작했지만
일부 경계 전환은 즉시 일어나지 않다가 다시 시도하면 성공했다.

## redacted 결과

| 항목 | S-B source | S-A destination | 확인 내용 |
| --- | ---: | ---: | --- |
| boundary/entry | intent `22` | entry `18` | intent와 entry 사이 `4` 차이 |
| capture lifecycle | activation/deactivation `11/11` | 해당 없음 | entry보다 activation `7` 적음 |
| pointer move | sent `1552` | injected `1519` | 전달 동작 확인 |
| button down/up | `7/7` | `7/7` | 일치 |
| wheel | sent `155` | injected `155` | 일치 |
| focus restore | restored `11`, failed `0` | 해당 없음 | v14 원인 counter도 모두 `0` |
| supervisor | active/failure `7/7` | active/failure `7/7` | 양쪽 `peer-session` |
| crash guard | remaining `0`, passed | remaining `0`, passed | 통과 |

source의 wheel은 `hookObserved=rawObserved=windowObserved=mapped=queueAccepted=sent=155`로 이어졌고
destination wheel도 `155`였다. capture activation/deactivation과 focus restore는 `11/11/11`로 닫혔으며
API 거절과 verification loss는 모두 `0`이었다. 따라서 이번 실행의 간헐적 증상을 wheel queue 손실이나
focus 복원 실패로 설명할 근거는 없다.

## 해석 한계와 후속 결정

`22-18=4`와 `18-11=7`은 control handoff의 두 구간에 구멍이 있음을 보여주지만 각 counter가 같은
generation을 가리킨다는 보장은 없다. `lastFailureComponent=peer-session`도 실제 원인보다 connection
close로 깨어난 reader를 먼저 기록했을 수 있다. 이 v14 aggregate만으로 not-ready drop, write outcome,
grant apply, source activation 또는 capture activation 중 하나를 원인으로 확정하지 않는다.

ADR 0024와 `agent-session/v15`가 boundary admission, request/grant/entry, source/capture activation과 local
interrupt 뒤 read를 분리한다. 다음 실행은 v15 clean artifact로 같은 왕복을 반복하고 양쪽
`controlFlow`, `supervisor.firstFailure`, `supervisor.failureCounts`를 비교한다. v15 실장과 CI는 이 문서의
하드웨어 판정을 소급해 통과로 바꾸지 않는다.
