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

## g152 handoff validation notes

g151·g152 계측은 경계를 넘을 때의 준비 비용(crossing)과 넘어간 뒤 연속 입력의 비용
(continuous)을 분리해서 본다. 같은 프레임 벡터라도 겹치는 span은 더하지 않고, 구간
bucket 최대값으로 p95나 전체 지연을 만들지 않는다. 다음 하드웨어 시험은 g152 fresh
agent로 A→B 이동을 두 번 하고, 두 번째 뒤에는 중앙에서 10초 연속 이동한 다음 B→A를
일반 조작으로 확인하고 양쪽 진단을 export한다. 실기기 PIN 창 제어와 실제 KVM 체감
개선은 아직 확인하지 않았다.

`StartupProfile`의 `loadMicros`/`loadMeasured`는 `WorkerInstalledValidation` 안에
포함되는 하위 관측이고, `KRD3` 48-byte envelope는 `KRD2` 40-byte에 8-byte
extension(4-byte big-endian load, 1-byte presence, 3-byte reserved)을 더한 것이다.
구형 reader는 `KRD3`를 받지 않으므로 보호 설치가 일치해야 한다.

변경2(구현): inputdesktop native handle의 object name이 `Default`인지 확인하는 작업을
handle 획득 시 정확히 한 번 수행하고 그 사실을 `desktopBinding.verifiedHandle`에 남긴다.
bool proof가 아니라 "Default name이 이 정확한 live handle에 속한다"는 handle 값을
기억하므로 handle이 교체되면 이전 proof는 재사용되지 않는다. `verifyName`은 새 질의 전에
기존 proof를 지우고, `openDesktop`은 name 검증 뒤 `UOI_IO`를 확인한 다음
`SetThreadDesktop`을 호출한다. `active`는 작업마다 `verifiedHandle == owned`와 fresh
`UOI_IO`를 다시 확인하며 activity TTL 캐시는 두지 않는다. `close`는 proof를 폐기하며
복원 실패 시 `ErrCleanup`을 반환한다. `Scope`는 그 OS thread를 재사용하지 않고 worker는
오류 경로로 종료한다. syscall handle은 private fixed lease object로 다루며 교체 시 새
검증이 필요하다. 추측한 외부 HWND 재사용과 권한 변경은 없다.

로컬 네이티브 마이크로벤치마크(`BenchmarkDesktopScopeValidation`, owned thread, 입력
주입 없음, `-benchtime=300ms -count=2`)에서 name-on-every-operation 4496/6518ns,
376B/6 alloc 대비 handle-name-once 2239/2448ns, 56B/2 alloc을 기록했다. 제거된 반복
name read만 비교한 격리된 desktop 검증 수치이고, 실제 KVM 동작·체감 지연이나 g151 66ms
문제의 해결을 뜻하지 않는다.

로컬 검증: inputdesktop, inputworker, wininput, residentagent, localapi 테스트와 scoped
vet을 통과했다(name-proof demotion 테스트의 desktop 재실행 포함). 빌드는 미실행이다.
