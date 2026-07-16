# Performance Budget

- Status: Draft

## Contract

Performance budgets must track latency, payload size, query count, cache behavior, bundle size, background jobs, and UNDECIDED project-specific thresholds.

## Required Evidence

- Source of truth: UNDECIDED
- Owner: UNASSIGNED
- Merge-blocking validation: VALIDATION.md
- Related checklist: CHECKLIST.md

## Review Blockers

- A change bypasses the source of truth.
- A change weakens validation or hides skipped checks.
- A change lacks failure, recovery, security, performance, or test evidence where relevant.

## Windows mouse source hot path

- 대상 부하: mouse 또는 Precision Touchpad의 relative packet 최대 1000Hz를 우선 검증 범위로 둔다.
- 현재 `WM_INPUT`은 `GetRawInputData` 한 번으로 읽고, 그 사이 쌓인 packet은 64KiB 재사용
  `GetRawInputBuffer`로 queue가 빌 때까지 drain한다.
- Raw Input buffer는 8-byte aligned이며 session 시작 시 한 번 할당한다. packet마다 goroutine,
  buffer, slice 또는 JSON을 만들지 않는다.
- 한 Raw Input mouse packet은 최대 11개의 고정 배열 action으로 정규화하며 move와 button 순서를
  기존 용량 4096 `SourceSession` queue에 보존한다.
- wheel은 low-level mouse hook message에서만 정규화한다. 1000Hz move callback은 wheel mapper와
  panic boundary를 호출하지 않으며 wheel callback도 blocking I/O 없이 queue offer까지만 수행한다.
- low-level keyboard/mouse hook, Raw Input, touchpad-capable capture window는 서로 다른 고정 OS thread에서 실행한다. Raw Input batch
  drain과 capture window message loop는 hook message loop를 막지 않아야 하며, 세 thread의 boundary observation만 짧게 직렬화한다.
  packet parse, boundary observation과 queue offer는 blocking I/O 없이 끝나야 한다. network write는
  기존 sender goroutine 책임이다.
- end-to-end pointer latency, batch 크기 분포, queue high-water mark와 1000Hz 지속 부하 여유는
  아직 `UNDECIDED`다. 실제 S-A/S-B `agent-session` 재실행 전에는 성능 통과를 주장하지 않는다.
