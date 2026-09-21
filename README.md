# Krasis

- Status: Draft
- Scope: general
- Repository Type: desktop-app
- Addons: cli-tool

Krasis는 Windows 노트북 두 대 사이에서 키보드와 마우스 입력을 넘기는 개발 단계 제품이다.
Go/Win32 입력 core, mTLS, crash guard, 상주 agent, 장기 기기 신뢰 저장소와 CLI를 구현했다.
제품용 `krasis-workspace.exe`는 agent 자동 준비, PIN 등록, 좌우 배치, 연결 관리와 클립보드 공유를
제공한다. `krasis-control.exe`는 기존 관리 GUI이며 상대 기기 업데이트 등 남은 작업에 사용한다.
서명된 설치 프로그램과 전체 실기기 안전성 검증은 아직 완료하지 않았다.

## 주요 경로

- `cmd/krasis-lab/`: bounded Phase 0 실험 CLI
- `cmd/krasis-control/`: 제품 연결 관리 GUI
- `cmd/krasis-workspace/`: Wails v3 Workspace 관리 UI
- `cmd/krasis-agent/`, `cmd/krasis/`: 상주 agent와 제품 CLI
- `docs/desktop/product-control.md`: 현재 GUI 기능과 검증 경계
- `internal/`: 상태 모델, transport, peer session, Windows input과 crash guard 구현
- `docs/product/02-spec.md`: 제품 범위 source of truth
- `docs/architecture/02-runtime-flow.md`: 현재 runtime 조립과 장애 처리 계약
- `docs/experiments/phase-0-windows-input-feasibility.md`: 실제 장비 검증 절차
- `AGENTS.md`, `CHECKLIST.md`, `VALIDATION.md`: agent 작업 및 검증 계약

## 현재 경계

- 이 코드는 상시 사용 가능한 완성 제품이 아니라 입력 안전성과 복구 가능성을 증명하는 실험 단계다.
- 네트워크와 입력 hot path는 Go agent가 소유하며 Win32 GUI와 분리한다.
- 실제 두 장비 hook, 절전·잠금, 고 polling-rate mouse와 보안 제품 간섭은 수동 Phase 0 evidence가 필요하다.
- peer authorization, private-key DACL, OS lifecycle와 single-instance agent는 구현돼 있다.
  구현·로컬 테스트 통과는 실제 장비의 장애 복구, 호환성 또는 서명된 릴리스의 증거를 대신하지 않는다.


## Repository Hygiene

.editorconfig, .gitattributes, and .gitignore are generated to keep line endings,
binary diffs, local files, build outputs, caches, and secret files under control.

## 구현 순서

개발용 GUI 개선과 실기기 검증을 병행한다. 제한된 베타 승인에는 실기기·설치·복구 증거가 필요하다.
상세 순서는 `docs/product/01-roadmap.md`, 남은 안전 위험은 `docs/product/03-risk-register.md`를 따른다.
