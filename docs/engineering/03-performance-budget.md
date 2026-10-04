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

## g153 준비 최적화 후보 검증

두 후보를 모두 제외한다. 단일 이미지 read buffer를 32KiB에서 128KiB로 키운 warm 격리
벤치의 이득(기본 128KiB 11.570~12.652ms, median 11.607 vs 기본 32KiB 12.394~15.155ms)은
실제 두 이미지 병렬 경로에서 19.813~36.615ms 대 11.416~13.008ms 회귀로 뒤집혔다. 다만
순차 before/after same-load 통제가 아니라 회귀 원인은 확정하지 않는다.

두 이미지 protectedHandle ACL 검사를 기존 hash goroutine 안으로 옮기는 병렬 안의 격리
측정은 actual installed fixed pinned object 2개에서 serial 24.455/24.771/25.209us,
parallel 36.976/37.839/49.824us(추가 goroutine 포함)였다. 비용이 극히 작고 병렬 안이
오히려 느려 채택하지 않는다.

두 후보 모두 runtime 자체 변경 없이 되돌렸다. 이는 g153 후보 판정 시점이며, 당시 제품
런타임 변경은 없었고 실기기 개선 효과도 측정하지 않았다.

세 번째 후보인 Windows CNG full SHA256도 미채택한다. warm 8MiB memory fixture를 32KiB
chunk로 full same digest를 해시한 500ms 3회 측정에서 Go 7.959/8.006/8.150ms(2 allocs,
160B) 대 CNG 8.396/8.406/8.419ms(263 allocs, ~8.4KiB)였다. hash·algorithm·native handle
cleanup은 모두 확인했지만, OS native CNG가 더 느리고 힙 할당이 많아 채택하지 않는다. 이
수치는 단일 메모리 digest 벤치라 파일 전체 설치나 g153 준비 실기기 증거가 아니다.

## g154 capture foreground pending-activation dispatch

g151은 NULL foreground에서만 PeekMessageW(PM_QS_SENDMESSAGE|PM_NOREMOVE) dispatch를
수행했다. g154는 SetForegroundWindow가 nil 성공이어도 실제 foreground가 capture window가
아닌 조건(이전 non-NULL foreground가 남은 verification-lost)에서 AttachThreadInput
fallback을 호출하기 전에, owner thread에서 같은 단발 PeekMessageW dispatch를 한 번 수행하고
fresh GetForegroundWindow를 다시 읽는다. NULL 전환도 같은 단발 dispatch를 쓰며, 취소는
dispatch 전후로 검사하고 실패는 fail-closed로 유지한다. posted input·custom command·
WM_QUIT를 제거하지 않고, 저장된 외부 HWND 신뢰나 알 수 없는 스레드 attach는 하지 않는다.

이 비용은 기존 `CaptureForeground` span 안에 포함되고 새 span을 추가하지 않는다.
fake-native 테스트는 이전 non-NULL 20에서 pending 10 dispatch로 실제 10이 되고(attach·sleep
없음, savedFocus 20 보존) 취소 시 attach를 금지하는지 확인한다. 변경 범위는 source capture
활성화 경로뿐이며 worker pipe·권한·캐시·중복 해시 worker는 그대로다. 실기기 개선 효과는 아직
측정하지 않았고 설치 검증 절차도 변경하지 않는다.

## 다음 작업: 중복 worker load hash 제거(2단계 부트스트랩)

중복 worker load hash 제거는 별도 2단계 부트스트랩으로 진행한다. child는 보호 메타데이터를
읽고 service exe를 기존 Go SHA/full hash로 직접 검증해 자기 이미지와 서비스 actual pipe
owner + SCM 상태·설정을 독립적으로 확인한다. 인증이 끝난 뒤에만 서비스가 이미 검증해
쓰기·삭제를 차단한 agent image의 exact file object duplicate를 owned child에 넘긴다.
child는 agent PID·토큰·session·pipe owner 검증을 유지하고 agent 재hash만 제거하는 설계다.
service 검증보다 핸들 신뢰가 앞서면 순환 신뢰이므로 거절한다. 인증 실패나 서비스 stop일 때
child와 dup handle을 정리하고 recovery receipt 회귀 테스트를 거친 뒤 별도로 배포한다.
g154에는 포함되지 않으며, 이 항목은 다음 작업 초안이라 설계 확정·구현·속도 개선을
주장하지 않는다.
