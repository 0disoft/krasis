# 로드맵

- 상태: Draft
- 책임자: cherr

## 구현 순서

### Phase 0: Windows 입력 안전성 검증

실행 명세: `docs/experiments/phase-0-windows-input-feasibility.md`

현재 판정: `IN_PROGRESS`. S-B Precision Touchpad에서 S-A로 이어지는 v13 wheel 경로는
`601 sent = 601 injected`, capture lifecycle `7/7`, injection/guard failure `0`으로 통과했다. 전체 Phase 0은
장애·lifecycle, home pair, 역방향과 장시간 gate가 남아 완료하지 않는다.

- 대상 노트북과 주변기기 목록 확정
- 물리 입력 캡처와 선택적 억제 검증
- `SendInput` 기반 주입과 합성 입력 재전달 방지 검증
- 비정상 종료, 절전, 잠금, 네트워크 단절 뒤 `ReleaseAll`과 `LOCAL` 복귀 검증
- 결과가 불안정하면 Go 경계를 유지한 작은 native 입력 모듈의 필요성 판단

종료 조건: 사용자를 입력에서 가둘 수 있는 실패가 재현 가능한 방식으로 차단되거나, 구현 중단 사유로 기록된다.

### Phase 1: 순수 상태 모델

- 세션, 제어권, epoch, 눌림 상태와 전환 상태 머신
- 중복, 역순, 유실, 재연결에 대한 모델 테스트
- OS와 네트워크 없이 `ReleaseAll` 및 `LOCAL` 수렴 검증

### Phase 2: 기기 신원과 페어링

- fingerprint·역할·capability·revocation의 atomic local trust store와 CLI는 구현됨
- 정식 local device identity와 제품 default trust store 경로
- 사용자 확인이 포함된 페어링
- key rotation과 alias·권한 변경 경로

### Phase 3: 전송과 제어권

- 인증된 연결과 입력 순서 보장
- 제어권 획득, 갱신, 취소와 재연결
- ADR 0005 coordinator, caller-injected heartbeat worker와 aggregate 송수신·gap Phase 0 evidence 계약은 구현됨; 제품 기본값은 미결정
- heartbeat interval과 timeout을 실제 pair congestion·절전 실험으로 결정
- 장애 주입 테스트

### Phase 4: Windows 입력 통합

- S-A/S-B 기본 pointer, button과 Precision Touchpad wheel에는 순수 Go와 얇은 Win32 adapter를 선택했고 구현함
- 경계 전환, 포인터 좌표, 키보드와 마우스 전달
- 실제 장비 호환성 및 장시간 검증

### Phase 5: 상주 agent와 CLI

- 단일 인스턴스, tray lifecycle, 로컬 제어 API
- 상태, 진단, 페어링, 연결 해제 명령 계약
- 단일 authenticated connection의 bidirectional session 조립 경계는 구현됨
- listen/dial adapter와 fail-safe reconnect supervisor는 구현됨
- bounded live-input `krasis-lab agent-session`과 로컬 긴급 해제는 구현됨
- 별도 Phase 0 crash guard, parent-kill subprocess 복구와 report gate는 구현됨
- `SendInput` zero·partial·unknown 확실성, 반복 press 보존과 crash guard protocol v2 cancel은 구현됨
- grant 전 cursor entry의 prepare·commit·conditional rollback과 write failure 보상은 구현됨
- Windows suspend/resume, session lock·disconnect·logoff, shutdown과 display change를 terminal
  cancellation으로 만드는 lifecycle owner와 v7 evidence gate는 구현됨
- 입력 주입 실패 확실성과 cursor rollback 결과를 aggregate-only로 검증하는 evidence gate는 구현됨
- 장소별 순차 수집을 합치는 `phase0-check` evidence gate는 구현됨
- heartbeat payload, liveness domain, optional timer worker, explicit live CLI policy와 privacy-bounded aggregate 계측은 구현됨
- v13 입력 경로는 lab 전용 복제품이 아니라 `agent.RunSupervisor`, `agent.RunSession`과 `wininput.SourceHook`의
  공용 core를 사용함; `krasis-lab`은 bounded 실행과 redacted report를 소유함
- 정식 agent executable, 단일 instance와 tray UI lifecycle은 미구현

### Phase 6: Wails v3 + Svelte UI

- 설정과 상태 투영
- 입력 처리 경로와 독립적인 시작, 종료 및 장애 처리

### Phase 7: 설치와 제한된 베타

- 서명된 설치물, 방화벽 안내, 제거와 데이터 삭제
- 네 대의 실제 노트북으로 반복 가능한 회귀 표준 확정

## 순서 변경 금지 조건

Phase 0과 Phase 1을 통과하기 전에 GUI, 설치 프로그램 또는 세부 프로토콜 최적화로 넘어가지 않는다.
