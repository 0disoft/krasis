# Krasis

- Status: Draft
- Scope: general
- Repository Type: desktop-app
- Addons: cli-tool

Krasis는 Windows 노트북 두 대 사이에서 키보드와 마우스 입력을 안전하게 넘기는 제품을 검증하는 Phase 0 저장소다. 현재 Go 기반 상태 모델, mTLS peer transport, Windows low-level hook와 `SendInput` adapter, crash guard, 증거 수집 CLI가 구현돼 있다. Wails v3 + Svelte UI, 정식 상주 agent, 설치 프로그램과 장기 신뢰 저장소는 아직 구현하지 않았다.

## 주요 경로

- `cmd/krasis-lab/`: bounded Phase 0 실험 CLI
- `internal/`: 상태 모델, transport, peer session, Windows input과 crash guard 구현
- `docs/product/02-spec.md`: 제품 범위 source of truth
- `docs/architecture/02-runtime-flow.md`: 현재 runtime 조립과 장애 처리 계약
- `docs/experiments/phase-0-windows-input-feasibility.md`: 실제 장비 검증 절차
- `AGENTS.md`, `CHECKLIST.md`, `VALIDATION.md`: agent 작업 및 검증 계약

## 현재 경계

- 이 코드는 상시 사용 가능한 완성 제품이 아니라 입력 안전성과 복구 가능성을 증명하는 실험 단계다.
- 네트워크와 입력 hot path는 Go agent가 소유하며 향후 Wails UI에서 분리한다.
- 실제 두 장비 hook, 절전·잠금, 고 polling-rate mouse와 보안 제품 간섭은 수동 Phase 0 evidence가 필요하다.
- peer identity 기반 authorization, Windows private-key DACL, OS lifecycle owner와 정식 single-instance agent는 아직 release blocker다.


## Repository Hygiene

.editorconfig, .gitattributes, and .gitignore are generated to keep line endings,
binary diffs, local files, build outputs, caches, and secret files under control.

## 구현 순서

GUI보다 session lifecycle, trust boundary와 실제 장비 검증을 먼저 끝낸다. 상세 순서는 `docs/product/01-roadmap.md`, 남은 안전 위험은 `docs/product/03-risk-register.md`를 따른다.
