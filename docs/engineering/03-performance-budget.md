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

## g156 독립 worker 서비스 이미지 검증(중복 agent 재hash 제거)

서비스는 기존 두 이미지 full hash 검증 pin을 그대로 유지한다. 자식(worker)은 보호 metadata와
service/input image만 full hash한 뒤 self 이미지·SCM 상태·설정·actual pipe owner를
독립적으로 검증하고, 서비스에 KRA1 ack를 보낸다. 그 뒤 서비스가 이미 검증해 쓰기·삭제를
차단한 agent image와 pipe를 exact owned child process handle로 `DuplicateHandle`해 KRB2
packet으로 전달한다. 자식은 fixed protected path의 exact volume/file identity를 확인한
다음에만 pending Installation을 채택하며 agent image를 다시 해시하지 않는다. agent
PID·token·session·pipe owner 검증은 그대로 유지한다. 실패/취소 시 job을 cleanup하고,
input ready 뒤에는 기존 recovery receipt 경로를 그대로 쓴다. worker_installation_windows.go,
bootstrap_windows.go, bootstrap.go, cmd/krasis-input/main_windows.go가 바뀌었다.

리뷰 blocker 수정으로 KRB2에 agent sha256 32바이트를 추가한다. generic `RunAgentLease`의
nil service 경로가 검증한 digest와 worker 보호 metadata의 digest를 결합하기 위함이고,
`decodeWorkerBootstrapHandles`는 전달된 agent digest가 기대한 protected agent digest와
다르면 거절한다. packet은 24바이트 handle/header + 32바이트 digest + 432바이트 lease =
488바이트다. g155 폴더는 로컬 빌드가 유효했지만 적용하지 않았고, 최종 리뷰에서 이 수정이
나와 덮어쓰기를 피하려고 g156 새 폴더로 빌드한다. g155 내부 후보는 적용하지 않는다.

`WorkerLoad` 계측은 이제 보호 metadata와 service-only hash를 재고 adoption 단계는
`WorkerAgentAuth` span에 포함된다. 이전 full load 계측과 직접 동등 비교가 아니다.

scoped Go 테스트와 vet은 통과했다. native service 설치·실기기 PIN·성능은 아직 미검증이고
빌드는 계획 단계다.

## g157 service pipe startup grace

`connectInstalledService`가 여는 fixed service endpoint만 `ERROR_FILE_NOT_FOUND`를 dial
시작 기준 최대 250ms 동안 20ms 간격으로 재시도한다. grace는 첫 open부터 세고 시도마다
초기화하지 않으며 감싼 deadline을 늘리지 않는다. `ERROR_PIPE_BUSY`는 기존 caller-bounded
wait(2초)를 그대로 유지하고, worker control pipe의 generic open은 이름이 없으면 즉시
실패하는 기존 동작을 유지한다. `ACCESS_DENIED`·`ERROR_PATH_NOT_FOUND`·
`ERROR_BROKEN_PIPE`와 authority/protocol 실패는 재시도하지 않는다.

open이 성공하면 SCM actual pipe owner와 metadata/hash auth는 그대로 수행한다. 이 대기는
lease/input을 보내기 전 연결 단계에서만 일어나고, 입력 재전송이나 backend fallback은 없다.
context 취소·deadline이 우선하고 성공 후 취소면 handle을 닫는다.

g156 실기기 exports에서 12전환 모두 ok였고 source 승인 median 137.204ms(g154 latest 12는
181.647), B load median 28.808ms(g154는 69.718)였다. g156에서 작업이 제거돼 이전 수치와
의미 범위가 다를 수 있다. B 연속 782 frame 중 15.436ms가 1건 있었다. export 직전
`pipe_open`/`native_2` 1건은 service 종료인지 준비 틈인지 미확인이고, A 로그에서 약
2.3초 자동 재연결을 확인했다.

검증은 native 임시 파이프 fixture로만 진행 중이며 제품 service 변경은 없다. 실기기 효과는
아직 확인하지 않았다.

## g157 native pipe owner query benchmark

`service_owner_bench_windows_test.go`는 임시 named pipe 한 쌍만 쓰고 제품 service·입력·권한은
건드리지 않는다. `runtime.LockOSThread`로 한 번 고정하고 warm PID를 확인한 뒤, `b.Loop`
안에서는 호출마다 `time.Now`/`time.Since`를 재고 배치 전후 `GetThreadTimes`의 kernel+user
합산(CPU 100ns 단위)을 기록한다. 300ms 3회에서 native owner 평균 6513/6665/6459 ns/op,
배치 CPU 합계 6230/6633/5525 ns/op, wall max 2.3288/2.5776/1.7051 ms였다.

이 수치는 임시 파이프 fixture의 호출 비용일 뿐, B 실기기에서 보인 초기 11.573ms 지연의
원인을 확정하지 않는다. 호출의 고정 비용이나 PID 캐시 필요성으로도 보지 않는다. CPU 값은
배치 합계라 개별 slow call의 CPU에 연결할 수 없고 Windows 계정 단위 정밀도를 갖는다. 참고:
[GetThreadTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getthreadtimes).

다음 단계는 bounded cycle probe 후보의 비용을 측정한 뒤 채택 여부를 판단하는 것이다. 별도
g158 빌드는 실행하지 않았다.

## g158 bounded cycle probes(진단 전용)

g158은 선택적 진단만 추가한다. native owner 질의와 fresh SCM 검사는 그대로 두고, 호출 전후
`QueryThreadCycleTime` 사이에 `runtime.LockOSThread`로 goroutine을 고정하며 unchanged native
owner 질의는 한 번만 수행한다(`internal/inputworker/pipe_owner_cycles_windows.go`의
`measurePipeOwner`, `internal/handoff/authority_cycles.go`). 고정은 스레드별 CPU 합계가
섞이지 않게 하기 위한 것이고 스케줄링을 바꿀 수 있다. cycles에는 counter/clock 오버헤드가
포함되며 주파수 변환은 하지 않는다. CPU 질의 실패·불가는 미측정으로 남고 auth 처리에는 영향이
없다.

probe는 collector당 최대 2048회로 제한하고 기존 12시도·120초 창을 그대로 쓴다. 재진입으로
횟수 제한이 초기화되지 않는다. 값은 first/worst
동일 벡터 필드 `pipeOwnerCycles`와 `pipeOwnerCyclesMeasured`에만 붙고 진짜 0은 absent와
구분된다. old diagnostic과 호환되며 optional numeric은 `1<<48` 이하다. raw PID·입력 내용·새
로그는 넣지 않는다.

prototype 벤치는 first base 5576/5545/5652ns 대 probe 7635/7634/7591ns, 실제
`measurePipeOwner`는 base 6215/7355/5975ns 대 probe 12968/9904/8017ns였다. 후보 예산에
10us 미만이 더해지지만 paired same-load 대조가 아니고 두 벤치 모두 thread-pinned이며 full
pipeline quota/export 비용은 벤치에 없다.

관련 패키지 테스트·native fixture·static vet은 통과했다. g158의 실기기 효과는 아직
확인하지 않았다. 앞선 native 벤치 커밋은 740be71이다.

## g159 one-shot unbound console preparation

The g158 exports created at 2026-10-04 14:47:50/54 UTC contain twelve matching
A-source/B-installed-receiver attempts, all complete and successful. A grant
median/max was 142.097/304.606ms. B ready-wait median was 123.262ms, launch
19.708ms, child-connect 92.952ms and load 31.997ms. These nested/local spans
must not be added. B's 763 non-first frames were all below 10ms. There were no
post-connect pipe/heartbeat failures or reconnects in this exported g158 window.

Four slow owner calls took 3.201–5.226ms wall with 68,400–102,060 cycles;
all 24 exported first/worst owner cycle readings ranged 56,340–123,840. This
suggests waiting/scheduling rather than proportional CPU work; cycles are not
converted to time and do not identify the wait cause. The exports include one
incomplete historical control log, and the bounded profiles cover A-to-B only.

GOMAXPROCS=1 and deferred source callback registration were tested using only
the input helper's invalid-mode exit, in alternating before/after order. Neither
showed a consistent startup latency gain; neither change was adopted. The
fixture performs no installation, elevation, input or service activity.

g159 moves a single child's preparation after successful prior recovery and
before the next lease, with a two-second, one-use, same-console lifetime. The
first or later cold handoff retains normal launch. KRD4 marks prepared startup
and its bounded age; launch/child-connect measurements may precede the current
attempt and must not be summed into its ready wait. Native pipe/job fixtures,
freshness, expiry-versus-claim and framing tests cover local behavior. Actual
two-PC latency and PIN behavior were pending when g159 was built.

### g159 device observations (2026-10-05 KST)

The new exports were created at 2026-10-04 15:53:48/53 UTC. Filtering both
devices to revision eb2ec89 produced twelve complete successful A-source and
B-installed-receiver profiles. Eleven receivers used prepared children, with
claim ages 3.321–946.204ms. No input connection/heartbeat failure or reconnect
was recorded after CONNECTED; B's earlier acquisition timeout precedes connection.

| Local diagnostic span | g158 median | g159 median |
| --- | ---: | ---: |
| A request-to-grant | 142.097ms | 29.186ms |
| B lease-ready wait, all twelve | 123.262ms | 8.072ms |
| B lease-ready wait, eleven prepared children | not applicable | 7.908ms |

The first cold receiver waited 131.685ms. One prepared receiver still waited
61.866ms, so preparation does not eliminate every delayed handoff. B's 520
non-first frames included one 11.101ms dispatch; the others were below 10ms.
Both installation receipts report success and B has sixty lease-ready records.
These are bounded software timings from consecutive runs, not a controlled
end-to-end pointer measurement. Historical control logs remain incomplete.
The user subsequently confirmed successful g159 PIN control on the real devices
(2026-10-05 KST). This is the user's direct observation; support export does not
collect PIN values or dialog contents. User-perceived latency and reverse-direction
profiles remain unconfirmed. No further runtime change follows from this snapshot alone.

## g160 near-boundary preparation hint

g160 adds an opt-in preparation hint before the crossing. The source hook enqueues
one timestamp when the pointer moves outward within 192px of the edge (single-slot
non-blocking queue; band clamped to a quarter of the screen width) and does not
re-arm until the pointer retreats beyond twice the band. The hint carries no
identifiers or geometry and is checked against local run state before any write.

Product endpoints advertise TLS ALPN `krasis/1.prepare` with the legacy
`krasis/1` fallback, so only negotiated peers send it. The control command is
version 4 kind 7 and the existing lease commands stay version 3. The receiver
accepts one hint, drops it after 500ms, enforces a 2s cooldown and a 300ms local
preparation wait, and requires a fresh trust/role/inbound permission check. The
installed agent and service authenticate the existing exact path, images, SCM and
pipe owner.

The fixed `KRP1` local packet carries zero authority. At most one unbound child is
retained for 2s in the same console session and can be claimed once; no input
transfers before a fresh normal lease is approved. Duplicate hints do not extend
expiry, slow/expired/missed requests use the existing cold fallback, and a failed
cleanup remains a service failure. Existing `preparedAgeMicros`/`prepared` diagnostics
are reused and the hint adds a fixed acknowledged/missed outcome; an
acknowledgement is not lease readiness.

g159's first cold handoff was about 132ms; g160's goal is only to move cold
preparation off the handoff. There is no real-device evidence yet, and no latency
improvement is claimed.

### g160 device observations

Support exports were created at A 2026-10-04T16:39:35Z and B 2026-10-04T16:39:28Z and
filtered to the full revision 3d04734c590051e5c5e6d94af38cab3290c68ff6. There is one
matching source run and receiver run, 12 profiles each, all end ok and incomplete
false. All 12 B startups are prepared=true with ages 2.943–1170.362ms. A
request-to-grant median/max was 28.918/134.225ms and B ReadyWait median/max was
7.187/7.864ms. The first A grant was 126.796ms and the first B OpenLock 96.643ms.

Attempts 4, 5 and 10 followed reentry gaps of about 6.6, 14.1 and 10.7s. Their A
grants were 100.060/134.225/79.303ms, B OpenLock 71.983/104.918/52.484ms and their
prepared ages 3.067/4.650/2.943ms. The new PrepareInput holds the same mutex as open
across the local preparation request, so preparation overlaps the handoff and its
waiting moved into OpenLock. Cold or idle handoff is therefore not solved even with
all Prepared=true. Overlapping spans must not be summed and the two devices' wall
clocks differ, so cross-device clocks must not be compared.

B recorded 1654 non-first frames: 88 below 1ms, 1552 in [1,10)ms, 14 in [10,100)ms and
0 at or above 100ms. The 20.562ms maximum matches the same sample's client_response of 20.247ms,
which includes worker, transport and scheduling; the root wait cause is unresolved.
The first input is excluded from the histogram. Both installation receipts are
result 0 with stage complete. After the latest CONNECTED there is no
connection/heartbeat failure or RECONNECTING in the export. B's five
acquire-network-timeout events precede CONNECTED during update/setup, not post-connect
losses. B's fixed preparation acknowledged 15 / missed 0 does not prove claim
readiness; the actual startup profiles prove prepared use. Each export has one
incomplete historical control log, the bounded profiles cover A-to-B only, and the
reverse direction and the new g160 PIN outcome remain unconfirmed.

The next direction is that preparation must lead the actual edge request by enough
time; an improved cold latency cannot be claimed yet. Field indices were verified
against handoff/profile.go (OpenLock 6, ServiceAuth 9, ReadyWait 12, Dispatch 28).
No new build or runtime test was required because this is evidence only.
