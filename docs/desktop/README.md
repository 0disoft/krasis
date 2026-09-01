# Desktop Boundary

- 상태: Phase 0
- 현재 artifact: Windows amd64 probe kit
- 목표 UI: Wails v3 + Svelte
- 제품 installer와 auto-update: 미구현

## 현재 구현

현재 desktop surface는 `krasis-launcher.exe`라는 작은 Win32 Phase 0 launcher다. probe-kit integrity를
확인하고 strict `launcher-config.json`을 편집하며 credential 준비, bounded `agent-session`, local
emergency stop와 aggregate report 복사를 조립한다. 이 launcher는 tray product, background service,
installer 또는 update client가 아니다.

Windows input, authenticated peer connection, lease, liveness, `PressedState`와 cleanup은 Go agent가
소유한다. launcher 종료나 future UI crash가 active input session의 안전 owner가 되어서는 안 된다.

## 목표 구조

Wails v3 + Svelte UI는 상태, peer, layout와 안전 상태를 표시하고 production agent의 local authenticated
API에 명령을 보낸다. UI process는 hook callback, ordered write, lease registry, input injection,
`ReleaseAll`과 reconnect loop를 소유하지 않는다. agent API가 구현되기 전에는 UI mock을 production
상태처럼 연결하지 않는다.

## OS와 배포 경계

CI는 현재 `windows-latest`에서 `krasis-lab.exe`와 `krasis-launcher.exe`를 빌드하고 Windows amd64
probe kit를 만든다. 이것은 제품 지원 선언이 아니다. Windows version과 CPU architecture floor는 네 대
device matrix의 Phase 0 결과를 검토한 뒤 결정한다.

installer, code signing, firewall rule, start-at-login, single-instance tray lifecycle, uninstall cleanup,
upgrade migration과 automatic update channel은 아직 없다. unsigned probe kit를 제품 installer처럼
배포하지 않는다.

## Local Data and Privacy

local trust store 계약은 ADR 0006, launcher config·report·credential 보존과 삭제는
`docs/desktop/local-data.md`, Phase 0 launcher 동작은 `docs/desktop/phase-0-launcher.md`가 소유한다.
자동 crash upload와 telemetry upload는 없다. report는 aggregate JSON이며 raw key, input text,
pointer trajectory, window title, username, hostname과 absolute local path를 포함하지 않는다.

## 권한과 보안

Phase 0 agent, launcher와 crash guard는 현재 사용자 권한으로 실행한다. elevated helper와 system
service는 없다. trust store와 credential directory는 Windows protected DACL 검증을 거치며 mismatch를
빈 신뢰 목록으로 가장하지 않고 실패한다. secure desktop과 높은 integrity target에 대한 입력 보장은
범위 밖이다.

## 아직 결정하지 않은 desktop 계약

| 결정 | 책임 | blocker | 결정 trigger |
| --- | --- | --- | --- |
| Windows/architecture support matrix | repository maintainers | #4부터 #7까지 물리 evidence 미완료 | #8 종료 |
| agent install scope와 autostart owner | repository maintainers | resident agent 미구현 | #13 architecture 승인 |
| installer format, signing과 firewall UX | repository maintainers | install layout와 publisher identity 미확정 | #16 착수 |
| UI-agent API authentication과 versioning | repository maintainers | agent command surface 미확정 | #13 완료 뒤 #15 |
| crash-report retention과 opt-in export UX | repository maintainers | production diagnostics schema 미구현 | #13 observability contract |
| update channel | repository maintainers | MVP에서 자동 update 제외 | post-beta 별도 ADR |
