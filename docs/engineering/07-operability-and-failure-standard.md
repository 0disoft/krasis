# Operability and Failure Standard

- Status: Draft

## Contract

Operability standard connects code changes to logs, metrics, traces, rollback, runbooks, health checks, incident response, and failure evidence.

## Required Evidence

- Phase 0 source of truth: `docs/ops/ci.md`, `docs/experiments/device-probe-runbook.md`,
  `docs/desktop/local-data.md`, `docs/adr/0004-phase-0-crash-guard.md`,
  `docs/adr/0007-windows-agent-lifecycle-owner.md`,
  `docs/adr/0031-peer-dispatch-and-input-queue-evidence.md`
- Owner: UNASSIGNED
- Merge-blocking validation: VALIDATION.md
- Related checklist: CHECKLIST.md

## Phase 0 Failure Evidence

- live-input 명령은 bounded duration과 명시적인 사용자 확인 없이 시작하지 않는다.
- supervisor, liveness, source queue, destination disposition, control-flow와 crash guard 결과는 개인 입력을
  복원할 수 없는 aggregate JSON으로 남긴다. 기존 report를 덮어쓰지 않는다.
- 실패 report도 원인 분석 evidence이므로 launcher나 CLI가 자동 삭제하지 않는다. 보존·삭제 경계는
  `docs/desktop/local-data.md`가 소유한다.
- connection, hook, lifecycle monitor 또는 guard failure는 입력 계속이 아니라 suppression 해제,
  `ReleaseAll`, connection close와 `LOCAL` 또는 terminal `FAULT` 수렴을 요구한다.
- hosted CI는 deterministic port와 Windows API mock을 검증하지만 실제 mouse, Precision Touchpad,
  foreground, Modern Standby와 공용 Wi-Fi를 재현하지 않는다. 실기 결과는
  `docs/experiments/results/`에 aggregate로 별도 기록한다.

## Current Operational Boundary

정식 background service, metrics·trace backend, dashboard, alert, installer rollback과 자동 report upload는
아직 없다. `.cache`, `build`, `dist`, `reports`와 `secrets`는 health state나 source of truth가 아니며,
삭제 가능한 범위와 보존할 evidence를 구분하지 않은 일괄 cleanup은 금지한다.

## Review Blockers

- A change bypasses the source of truth.
- A change weakens validation or hides skipped checks.
- A change lacks failure, recovery, security, performance, or test evidence where relevant.
- A cleanup path can erase the current failure evidence or an in-use credential pair without explicit user intent.
- A hosted-CI pass is presented as proof of physical Windows input behavior.
