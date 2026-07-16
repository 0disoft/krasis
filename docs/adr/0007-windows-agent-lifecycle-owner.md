# ADR 0007: Windows agent lifecycle을 fail-closed owner가 소유한다

- 상태: Accepted
- 결정일: 2026-07-13
- 범위: Phase 0 `agent-session`과 향후 상주 agent의 Windows session lifecycle

## 배경

network와 peer session cleanup은 구현됐지만 Windows 절전, 복귀, session 잠금·분리·로그오프,
시스템 종료와 display topology 변경을 직접 감지하는 owner가 없었다. 이 상태에서 process가 계속
살아 있으면 resume 뒤 낡은 lease, suppression 또는 화면 geometry를 재사용할 수 있다. crash guard는
agent 비정상 종료 뒤 destination의 pressed input을 해제하지만 살아 있는 agent의 OS lifecycle
판단까지 대신하지 않는다.

## 결정

- `winlifecycle` monitor는 숨겨진 top-level window와 전용 OS thread의 message loop를 소유한다.
- `WM_POWERBROADCAST`, `WM_WTSSESSION_CHANGE`, `WM_QUERYENDSESSION`, `WM_ENDSESSION`과
  `WM_DISPLAYCHANGE`에서 안전에 영향을 주는 event만 정규화한다.
- suspend, resume, session lock, console·remote disconnect, logoff, shutdown과 display change 중
  첫 event는 현재 agent context를 취소하는 terminal transition이다.
- resume에서 기존 session을 자동 복구하지 않는다. connection, lease, suppression과 geometry를
  모두 버리고 다음 명시적 실행에서 다시 진단하고 rearm한다.
- monitor가 event 없이 종료되거나 알 수 없는 event를 전달하면 fail-closed runtime failure다.
- Phase 0 process owner는 lifecycle context, connection source와 crash guard를 한 번만 등록한다.
  종료 시 새 session을 막고 source를 닫은 뒤, supervisor의 bounded cleanup이 끝난 상태에서 crash
  guard를 disarm하고 마지막으로 OS monitor를 닫는다. 중복 Close는 실제 child owner에 한 번만 전달한다.
- 현재 `krasis.lab.agent-session/v13` report는 monitor 시작·정상 종료, 정규화된 event와 explicit rearm
  필요 여부만 기록한다. session id, window handle, 사용자 이름과 원시 Windows message는 기록하지 않는다.
- lifecycle event는 안전 종료에는 성공했더라도 Phase 0 pair evidence 성공이 아니다. stable failure
  code와 `stopReason=lifecycle_transition`, exit `1`을 남기며 checker는 해당 report를 거부한다.

## 결과

살아 있는 agent가 OS lifecycle 변화 뒤 낡은 권한과 입력 상태를 계속 쓰는 경로를 차단한다. 대신
일시적인 display 변경이나 잠금도 실험을 종료하므로 사용자가 다시 실행해야 한다. 숨겨진 window가
Windows message를 받지 못하는 secure desktop, process freeze, 강제 전원 차단과 firmware별 Modern
Standby 동작은 이 결정만으로 증명되지 않으며 실제 장비 fault-injection이 필요하다.
