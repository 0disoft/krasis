# 2026-07-17 S-A/S-B v15 local context menu A/B

- 판정: `REPRODUCED` — gram13 local context menu가 source capture activation을 결정적으로 차단
- 전체 Phase 0 판정: `IN_PROGRESS`
- 장소: 스터디카페
- source: `S-B`, `connect`, `LEFT`
- destination: `S-A`, `listen`, `RIGHT`
- agent schema: `krasis.lab.agent-session/v15`
- build revision: `1a25f16b75ff3d6b5c76ca905e36810bdf7ebe24`
- build modified: `true`

## 통제 조건

같은 장비 배치와 v15 modified build에서 두 실행을 분리했다. 첫 실행은 gram13 local menu를 열지 않고
14회 왕복했다. 두 번째 실행은 gram13에서 context menu를 연 상태로 8회 경계 진입을 시도했다. baseline
초반에 HP15에 연결된 mouse를 살짝 건드렸으므로 pointer 개수 자체는 성능 비교에 사용하지 않고 control
handoff 단계의 성공·실패만 비교한다.

## A/B 결과

| 단계 | menu 없음 | gram13 menu 열림 |
| --- | ---: | ---: |
| S-B boundary queued/admitted | `14/14` | `8/8` |
| S-B request/grant/apply/source activation | `14/14/14/14` | `8/8/8/8` |
| S-B capture attempted/succeeded/failed | `14/14/0` | `8/0/8` |
| S-B revoke received | `14` | `0` |
| S-A entry committed | `14` | `8` |
| S-A injected input | pointer `762` | `0` |
| S-A revoke write | `14` | `0` |
| guard | 양쪽 clean | 양쪽 clean |

menu 없음 실행은 capture activation/deactivation과 focus restore도 `14/14/14`였고 capture failure는 0이었다.
menu-open 실행의 source failure 분포는
`source-capture/capture-activate/activation_failed=8` 하나뿐이었다. destination은 remote EOF 7건과
peer-liveness component failure 1건을 남겼지만 이는 source가 activation 실패로 연결을 닫은 뒤의 결과다.

## 결론과 한계

`14/14` 대 `0/8`은 간헐적 Wi-Fi나 boundary detector보다 local Windows menu loop를 직접 원인으로
지목한다. 기존 대표 `failureCode=insufficient_liveness_evidence`는 session이 capture 전에 반복 종료돼
heartbeat 표본이 없어진 2차 증상이었다.

v16은 request 이전 `GetGUIThreadInfo` menu admission guard와 `blockedLocalMenu` evidence를 추가한다. 이
결과는 v16 구현의 실제 장비 통과 증거가 아니다. clean v16 build에서 menu-open 시 S-B request 0과 S-A
entry 0을 확인해야 한다. 또한 query 직후 menu가 열리는 경합과 menu 이외의 capture activation 실패는
two-phase handoff 전까지 남는다.
