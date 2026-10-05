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

## g161 pointer-position approach preview

g161 predicts the approach earlier than the g160 last-moment hint. g160's first/idle
attempts measured grants of 79–134ms with a preparation-mutex wait (OpenLock) of
52–105ms, so the preparation still overlapped the handoff. The new preview derives
speed from recent physical pointer X positions, not raw mouse counts, and is local,
serialized and O(1) per move.

The baseline band is `min(192px, width/4)`; the projected region is about 150ms
ahead, capped at `3/4 width`. Samples closer than 4ms are coalesced, the anchor
advances no more often than every 16ms, and the estimate resets on a sample older than 50ms, a
reversed clock, a reversed physical direction or an out-of-display position. The
sticky approach threshold stays until an actual retreat beyond `band + configured
rearm` (capped at `width-2`), so a falling speed estimate cannot rearm the same
outward approach.

The existing single-slot timestamp queue, 500ms freshness, 2s rate limit, 2s unbound
one-claim same-console lifetime, independent authorization/bootstrap and 300ms local
wait are unchanged; there are no new wire or service changes, and no coordinates are
logged or sent. These are conservative tuning constants and the hardware effect is
not yet verified. Tests cover both directions, slow or raw-counts-only input,
stale/reversed/outside/duplicate samples, sticky rearm, narrow-screen reachable reset
and the existing session integration. No measured speedup is claimed; g161 real-device
cold latency, continuous jitter and PIN regression remain pending.

### g161 device observations

Exports were created at A 2026-10-04T17:22:28Z and B 2026-10-04T17:22:24Z and filtered
to revision fdb71781c9442afefe316cf4569fcaca538e31f0. There are twelve A-source and
twelve B-installed-receiver profiles, all end ok with incomplete false. All twelve B
startups are prepared=true with ages 0.862–1066.792ms. OpenLock is present in the
measurement mask and records 0 microseconds in every B profile. The g160 first
OpenLock was 96.643ms, with delayed reentry values of 52.484–104.918ms.

| Local diagnostic span | g160 median / maximum | g161 median / maximum |
| --- | ---: | ---: |
| A request-to-grant | 28.918 / 134.225ms | 33.285 / 107.730ms |
| B lease-ready wait | 7.187 / 7.864ms | 8.324 / 25.390ms |

The first g161 profile had A grant 27.112ms, B ReadyWait 7.791ms and prepared age
68.344ms. Attempt 4 followed a 10.129s source reentry gap with grant 34.486ms,
B ReadyWait 9.354ms and prepared age 346.397ms. These observations support earlier
preparation removing the measured OpenLock wait in this run. The overall grant median
and ReadyWait did not improve, and consecutive runs are not a controlled comparison.

Attempt 6 still had A grant 107.730ms after a 1.083s source reentry gap. Its B
PeerAuthorize/AuthorizeTotal were 8.730/17.708ms, ConnectTotal/ReadyWait were
8.977/7.073ms and prepared age was 1066.792ms. Source RequestWrite was 0.566ms and
GrantCoordinator recorded 0 microseconds. The earlier preparation-mutex wait is absent
from this attempt; the remaining source grant delay has no established cause.
Transport, dispatch and scheduling need separate evidence before selecting a fix.
Overlapping spans must not be summed and the devices' wall clocks must not be
subtracted to infer a network duration.

The first source activation recorded CaptureActivate 40.567ms and first Dispatch
36.597ms; these spans are separate from non-first frame statistics. A's 541 non-first
frames were 535 below 1ms and six in [1,10)ms, maximum 1.550ms. B's 527 non-first
frames were 54 below 1ms and 473 in [1,10)ms, maximum 3.760ms. Neither device recorded
a non-first dispatch at or above 10ms. This measures software dispatch, not visible
pointer motion or total end-to-end latency.

After the latest CONNECTED neither export records a connection/heartbeat failure,
timeout or RECONNECTING. B's five acquisition timeouts precede connection. Both
installation receipts are result 0 with stage complete. Each export has one incomplete
historical control log; the bounded profiles cover A-to-B only. A slow approach past
the two-second preparation expiry, reverse-direction profiles, perceived jitter and
g161 PIN control remain unconfirmed. Evidence documentation needs no new build or
runtime change.

The user subsequently confirmed successful g161 PIN operation on the devices
(2026-10-05 KST). This is direct operator evidence; the export records no PIN values
or dialog contents. The timing and slow-approach limitations above remain open.

## g162 additive handoff exchange detail

g161's remaining outlier is A grant 107.730ms with B AuthorizeTotal 17.708ms,
OpenLock 0 and ReadyWait 7.073ms. g162 adds optional `exchange` detail to separate
successful request-write completion to full grant read from grant-read to matched
grant handling, plus receiver request-read to handling and successful grant write.
The four spans use local `time.Now` endpoints; receiver spans overlap and source
wait includes peer work, network and local read-loop scheduling. They cannot
isolate RTT or be summed into an end-to-end pointer latency. Peer payloads, ACKs,
the 39-stage vectors and PIN authority are unchanged. The
[exchange contract](../experiments/handoff-profile.md#g162-additive-exchange-detail)
defines presence masks, bounds and overlap handling. Local tests cover reordered
callbacks, stale association, expiry, limits, policy-clock separation and numeric
export. g162 is diagnostic only; device latency and PIN behavior remain pending.

### g162 device observations

Exports were created at A 2026-10-04T17:52:53Z and B 2026-10-04T17:52:46Z and filtered
to revision 7be3159e385cedb3700b1e2de7fe465b5980dd46. There are twelve source and twelve
installed-receiver profiles, all end ok with incomplete false. Every source exchange
mask is 3. Grant-read to matched handling records 0 microseconds in all twelve as a
present clock reading, not an absent observation. Request-write completion to grant
read has median/max 29.703/80.579ms. A grant median/max is 29.9445/81.127ms.

Every receiver exchange mask is 12. Request-read to handler records 0 microseconds
except attempt 3 at 0.661ms. Request-read to successful grant write has median/max
19.3795/66.744ms. The two longest A grants coincide with nonzero receiver OpenLock:

| Attempt | A grant | A write-complete to grant-read | B read to grant-write | B OpenLock | B ReadyWait | B prepared age |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 81.127ms | 80.579ms | 66.744ms | 44.634ms | 8.970ms | 4.851ms |
| 7 | 50.036ms | 50.036ms | 37.678ms | 18.725ms | 7.850ms | 3.116ms |

B AuthorizeTotal was 66.164/37.103ms on those attempts. The other ten OpenLock
observations record zero. All B startups are prepared=true with ages 3.116–924.572ms;
ReadyWait median/max is 8.021/10.084ms. Thus the two longer source waits occur before
the grant is fully read, and receiver lock waiting has reappeared. The zero OpenLock
result in g161 was specific to that sample, not a general elimination of contention.
Static inspection shows PrepareInput holds the same input.mu as open across the local
preparation request. The current profile does not identify the mutex holder, so
preparation overlap remains a candidate rather than a proven owner attribution.

A's 345 non-first frames are all below 1ms, maximum 0.752ms. B's 337 non-first frames
include seventeen below 1ms and 320 in [1,10)ms, maximum 3.424ms. A's first activation
records CaptureActivate 36.112ms and first Dispatch 31.480ms. These overlapping spans
must not be summed. Neither export records failure or RECONNECTING after the latest
CONNECTED. B's eight acquisition timeouts precede connection. Both installation
receipts are result 0 with stage complete; each export has one incomplete historical
control log.

The first and last profile emissions are about 5.8s apart and the largest source
reentry gap is 1.033s. The twelve-attempt quota was consumed during rapid reentries;
no ten-second idle reentry was captured, and approach speed is unclassified. Only
A-to-B profiles are covered. g162 PIN behavior remains unconfirmed; the earlier user
confirmation applies to g161. Source wait includes peer work, transport and local
reader scheduling, so no pure network RTT is derived by subtracting durations or
cross-device clocks. The next investigation is preparation contention and controlled
first/idle coverage. This evidence update changes no runtime or build.

## g163 bounded preparation-lead tuning and lock overlap

The pointer-position preview lead changes from 150ms to 250ms. The baseline 192px
band (quarter-width cap), three-quarter-width prediction cap, sticky retreat rearm,
4/16/50ms sampling, 500ms hint freshness, two-second cooldown and two-second
one-claim unbound child remain unchanged. Authorization, lease checks, worker lifetime
and mutex ownership are preserved. This is bounded tuning motivated by g162's
44.634/18.725ms receiver OpenLock observations; it does not establish their owner.

Optional preparationLockMicros/preparationLockMeasured detail records the overlap of
OpenLock with the latest completed preparation lock interval. This is a lower bound
on overlapping preparation ownership, not a complete mutex-owner history. It must
not be summed with OpenLock. Raw timestamps stay in memory. Simply releasing the
agent mutex would leave preparation serialized in the service and could relocate
the wait without reducing it, so that approach was not adopted.

Local tests cover earlier hints for moderate physical motion, retained width caps and
sampling behavior, clipped overlap, numeric validation/export and shutdown joining
preparation before releasing installation pins. The existing slow-approach limitation
remains: a hint can expire before crossing without another hint until retreat.
Device latency, controlled first/idle coverage and g163 PIN regression remain pending.

### g163 device observations

Exports were created at A 2026-10-04T18:19:38Z and B 2026-10-04T18:19:33Z and filtered
to revision 880dde99aa070cc0cb52defc8db7081e7861671b. Twelve source and twelve
installed-receiver profiles all end ok with incomplete false. Exchange masks are
3/12 throughout; source grant-read to matched handling records a present zero in all
twelve. A request-to-grant median/max is 34.1915/150.797ms.

The first receiver provides direct overlap evidence: OpenLock and
preparationLockMicros both record 82.019ms with preparationLockMeasured true. The
latest preparation's locked interval overlaps the whole measured OpenLock. A grant
is 111.701ms; B AuthorizeTotal is 104.139ms, prepared age 4.714ms and ReadyWait
8.598ms. The remaining eleven OpenLock/overlap observations are measured zeros.
The overlap remains a latest-interval lower bound and must not be added to OpenLock.

Eight startups use prepared children and four use cold startup. The latter have
valid startup detail with the optional prepared bool omitted, which decodes as
false under the existing Go schema:

| Attempt | A reentry gap | A grant | B ReadyWait | B preparation-lock overlap |
| --- | ---: | ---: | ---: | ---: |
| 5 | 7.980s | 136.346ms | 118.170ms | 0ms, measured |
| 6 | 9.622s | 142.665ms | 116.326ms | 0ms, measured |
| 7 | 8.401s | 150.797ms | 126.555ms | 0ms, measured |
| 10 | 2.285s | 142.200ms | 118.427ms | 0ms, measured |

Prepared ReadyWait ranges 6.443–8.598ms; the overall median is 8.2125ms. Excluding
the first startup, prepared ages range 265.845–1704.496ms. Two preparation
acknowledgements appear near the first attempt and about four seconds later; none
appear during the subsequent idle cases. This does not identify the exact sender or
receiver drop path. Static inspection shows sticky hint rearm requires physical
retreat, while sender/receiver cooldowns and the unbound child's lifetime are two
seconds. The 250ms lead has not solved either the observed first preparation overlap
or the cold fallback after idle. These trials are not a controlled before/after run.

A's 653 non-first frames include 650 below 1ms and three in [1,10)ms, maximum 1.482ms.
B's 644 include 56 below 1ms and 588 in [1,10)ms, maximum 7.172ms. First source
CaptureActivate/Dispatch are 29.419/22.234ms and overlap. Neither export records a
failure or reconnect after latest CONNECTED. B's nine acquisition timeouts precede
connection. Installation receipts report result 0, stage complete; each export has
one incomplete historical control log. The profile emissions span about 44 seconds
and include roughly eight-to-ten-second reentry gaps. Approach speed remains
unclassified; coverage is A-to-B only and g163 PIN behavior is not operator-confirmed.

The next investigation is bounded hint renewal after a completed crossing or expired
preparation, plus first-approach anticipation. Fresh lease authorization, recovery,
one-claim child ownership and resource bounds must remain intact. No cross-device
clocks or overlapping spans are added to infer RTT or total pointer latency. This is
an evidence-only update; no new runtime build follows from the record itself.

## g164 bounded preparation-hint renewal

g164 renews the preparation hint without requiring a retreat. `preparationBoundary`
keeps a `sentAt` timestamp and, after the hint lifetime plus the existing 2s rate
(`preparationHintRenewal` = 2s + 500ms), a fresh outward physical pointer move inside
the eligible band re-emits one hint even if the pointer never retreated past the rearm
threshold. The retreat rearm, the 250ms lead, the band caps and the 4/16/50ms sampling
are unchanged.

There are no timers or background renewals; renewal still needs an actual outward move.
Authorization, the 2s one-claim unbound child TTL, maximum one child, and the 2s sender
and receiver gates are unchanged. The motivation is the g163 idle cold starts; the first
unknown-speed approach and the preparation-mutex overlap remain unresolved.

Tests cover both directions, the renewal threshold, repeat suppression, and idle and
ineligible/reversed-time cases. Device latency and PIN behavior are pending, and no
reduced latency is promised.

### g164 device observations

Diagnostics were created at A 2026-10-04T18:44:43Z and B 2026-10-04T18:44:38Z and
filtered to the exact revision 25f81f1f7e3cbe6b5e3f53882842c457f113cf57. There are 12
source and 12 installed-receiver profiles: 10 ok/complete and 2 error/incomplete on each
side (attempts 7 and 8). All 12 receiver startups are prepared true, with ReadyWait
min/median/max 7.005/7.828/11.076ms.

Idle A gaps at attempts 2/3/4/5 were 9.746211/9.469821/11.034118/11.953330s and B
ReadyWait 7.005/7.900/7.756/11.076ms. Against the g163 idle cold observation of
118–127ms these are now prepared, though this is not a controlled latency guarantee.
Positive overlap appears on B attempts 2/4/5/6 with OpenLock
42.569/34.762/34.998/110.636ms and measured preparation overlap
42.569/34.762/34.455/110.636ms; the other eight are measured zero (absent numeric field
with its presence flag). All prepared ages span 3.448–1396.869ms.

A request-to-grant over the 12 attempts was min/median/max 25.090/32.9255/145.194ms and
CaptureActivate 3.786/33.089/129.419ms. A's 716 non-first frames were [689, 26, 1, 0, 0]
with a 15.120ms worst; B's 708 were [73, 635, 0, 0, 0] with a 9.063ms maximum.

A's error attempts 7 and 8 coincide with
activation_capture_foreground_missing_before_show at 18:44:15.783Z and 18:44:17.370Z;
capture rollback measured 1.532/1.166ms, and the actual absent-foreground root cause is
unconfirmed. A logged boundary_orchestrator_failed with RECONNECTING at
18:44:10.9336585Z and CONNECTED at 18:44:13.1698384Z (a 2.236s process-local event
interval); B logged remote_eof with RECONNECTING at 18:44:10.7542015Z then CONNECTED at
18:44:13.0311542Z. No new disconnect follows the final CONNECTED even though the failed
transitions 7/8 come after it. B recorded 10 network acquisition timeouts before the
first CONNECTED and one TLS failure during the reconnect. User intervention or PIN must
not be equated with the cause, which is unknown.

Installation receipts are result 0/complete and each export has one incomplete historical
log. B shows 10 prepared acknowledgements and 1 missed. The user states PIN works on
g164; the exact PIN actions are not separately confirmed. Coverage is A-to-B only and the
12-attempt quota was consumed over about 61s; no cross-device clocks were subtracted and
overlapping spans are not summed. The next priority is diagnosing the remaining
preparation-lock/capture readiness and the boundary_orchestrator failure; this is
evidence only with no build or runtime source change.

## g165 prepared-hint commit ordering and activation dispatch

A read-only GUI inspection failure marker now rejects only the local boundary intent
while keeping the authenticated session, with the original
ControlFlowBoundaryAdmissionFailed counter; guard, cancellation and unmarked failures
remain fatal. The g164 boundary disconnect origin remains unconfirmed.

`foregroundAfterTransition` dispatches the sent activation messages and a fresh capture
request before any polling delay, so no polling sleep occurs when a pending activation
completes immediately; a persistently NULL foreground still spends the five 10ms poll
waits. The ownership validation, cancellation checks and the no-unknown-thread-attachment
rule are unchanged.

Preparation runs under the same shutdown/pin mutex but releases it before the diagnostic
receipt callback (b85bdec), so the measured preparation-lock interval excludes that
callback; the service synchronous launch wait and the prepare RPC stay under the lock.
Preview eligibility is separate from the emission commit (06cd7c1): `sentAt`/rearm
suppression applies only after the bounded single-slot queue accepts, so a full or failed
queue retries only on a fresh physical move inside the outward band under the 2.5s
renewal, with no timer and no lost full-queue cooldown.

AgentGeneration is 165, and the nine-package desktop Go tests with vet -unsafeptr=false
passed with exit 0; a build is not claimed. Tests cover cancellation and fatality of guards, a next
intent on the same connection, no polling sleep when pending activation completes
immediately, bounded refusal on a persistently NULL foreground, a blocked receipt
callback holding no lease or pin, and full-queue initial/renew retries. Device delay and
PIN are not verified on g165; the g164 PIN check was user-passed, and real ~110ms
preparation and a missing foreground remain possible.

### g165 device observations

Exports were created at A 2026-10-05T07:48:32Z and B 2026-10-05T07:48:23Z and filtered
to the exact revision bbaf3aeee1d9cdb88a74c7d5e4ba061d049a98d5. Of 12 A-source and 12
B-installed-receiver profiles, 5 are ok/complete (attempts 1/2/3/4/11) and 7 are
error/incomplete (attempts 5/6/7/8/9/10/12). All 12 B startups are prepared true with
ages 4.743–1405.859ms and ReadyWait min/median/max 6.633/7.306/7.923ms.

OpenLock overlap is measured in all 12: attempt 4 is 24.717ms, exactly the same
preparation-lock value, and the other eleven are zero. The prior g164 maximum of
110.636ms is observational only and not a controlled improvement. A request-to-grant was
min/median/max 23.891/31.9085/246.206ms. Successful CaptureActivate was
3.873/7.083/22.477ms, while failed attempts were 55.888/57.629/76.440ms with fallback
52.046–64.564ms and rollback 1.045–1.843ms.

A logged 23 activation_capture_foreground_missing_before_show codes between
07:47:46.3604603Z and 07:48:20.8675897Z; 7 match the failed profiles and 16 occur after
the quota was consumed. The 12 profiles span 25.783287s on A and 25.8048999s on B and are
all A-to-B. The NULL foreground is observed before the show and carries the failure
classification; why it was absent is unknown. Existing restoreFocus/deactivateWindow
logic does not provide evidence of the preceding focus transition in this export.

A's 362 non-first frames were [361, 1, 0, 0, 0] with a 1.049ms maximum; B's 352 were
[34, 318, 0, 0, 0] with a 7.200ms maximum. A CONNECTED at 07:47:25.9698826Z and B at
07:47:24.9878862Z, and neither export shows a reconnect, disconnect or session stop after
CONNECTED. B's 40 network timeouts, 1 disconnect requested and session stop all precede
the initial CONNECTED and are not counted as connected-test failures. B recorded 35
lease_ready, 15 prepare acknowledgements and 0 preparation-missed, connect-failure or
heartbeat-failure events. Both receipts are result 0/complete, and each export has one
incomplete historical log with no current-revision events inside it.

The user has not confirmed g165 PIN behavior (g164 success is previous). Click-mediated
recovery is unconfirmed for this run. The next diagnosis focuses on capture foreground
loss and restoration while retaining fresh ownership checks. These timing observations
do not isolate the effect of the g165 changes. This is evidence only with no build or
runtime change.

## g166 optional focus-state diagnostics

`Summary` gains an optional `Focus`, schema 1. `states[4]` covers beforeShow,
beforeRestore, afterRestore and afterHide with values 0 unobserved, 1 NULL, 2 capture,
3 saved original and 4 other, plus an `observed` 4-bit mask. A `restore` result is 1
skipped, 2 succeeded, 3 API rejected or 4 verification lost; `postHide` is 1 recovered,
2 NULL or 3 capture retained; `resultMeasured` holds their two bits. `micros[2]` carries
the restoreFocus and hideAfterFocusRestore durations with their own measured bits.
A profile must have at least one observed, result or duration bit, and values respect
the 120-second window.

Each field keeps its first write on the original source attempt, the existing
12-attempt/120-second collector bound applies, and `Retire` deep-copies the profile so
later updates cannot change an emitted record. The extra native focus queries are
alive-gated by `FocusEnabled` and run outside the collector lock. No focus policy, TTL,
lease or wire behavior changes, and no handles or window/application metadata leave the
owner thread.

AgentGeneration is 166. The nine-package desktop Go tests and vet with
unsafeptr=false passed with exit 0, including native return/rollback context,
late-owner isolation, post-hide NULL observation, bounded schema export and private
field removal. Device delay and PIN behavior are unverified on g166. The g165
foreground-loss findings motivate the preceding-return investigation.

### g166 device observations

Exports were created at A 2026-10-05T08:51:05Z and B 2026-10-05T08:50:55Z and filtered to
the exact revision 8ac5ee046ff3bac1226f9ef14be84fac017971ae. A has 12 source and B 12
installed-receiver profiles; handoff end=ok is observed in attempts 1, 5, 6, 7, 8, 9, 10
and end=error in 2, 3, 4, 11, 12.

All A profiles carry Focus schema 1 with observed 15, measured 3 and resultMeasured 3.
Handoff end=ok attempts 1, 9 and 10 have states [3,2,2,1] with restore 3 (APIRejected) and postHide 2
(NULL), and their restore durations are 3.799/28.194/4.737ms with hide durations
7.751/8.382/31.421ms. Failures have states [1,1,1,1] with restore 1 (skipped) and
postHide 2. Successes 5..8 have states [3,2,3,3] with restore 2 (succeeded) and postHide
1 (recovered), with restore durations 191.954/399.587/649.035/285.129ms. Sequences
1->2..4 and 10->11..12 support a failed restore followed by a NULL next activation, while
9->10 starts non-NULL so the intervening recovery cause is unobserved. Twelve
missing-before-show codes were logged, five matching failed profiles and seven after the
quota was consumed.

Every B profile is prepared true with ages 1.120–1606.273ms. OpenLock is measured in all
twelve: attempt 8 is 84.285ms, exactly the preparation-lock value, and the others are
zero. B ReadyWait is min/max 6.428/93.971ms over the 12 profiles. A's histogram is
[364,45,0,1,0] over 410 frames with a 456.566ms worst at attempt 7; B's is
[42,351,2,0,0] over 395 with an 11.365ms maximum at attempt 5. A SourceDeactivate reached
1425.024ms at attempt 7; nested overlapping spans are not additive.

A CONNECTED at 08:50:09.074549 and B at 08:50:08.0245578, with no later disconnect,
reconnect or session stop. B's 11 network timeouts, 1 disconnect and session stop all
precede the connection, and after CONNECTED B recorded 32 leaseReady, 12 prepareAck and 1
prepareMiss at 08:50:08.3504985, before the first profile. Each support export has one
incomplete historical log with no current-revision events, and both receipts are
result 0/complete. PIN and click recovery are not confirmed for g166. Timings are
process-local; clock values from different devices are never subtracted.

The next work is to preserve the existing restore fallback enum and its presence per
attempt (the current Focus loses the exact reason) without blind saved-handle
reactivation; a savedFocus reset is not evidenced as the cause. This is evidence only,
with no test, build, runtime or API change.

## g167 restore-fallback cause diagnostics

`Focus` schema 1 gains an additive `restoreFallback` (`uint8`) plus
`restoreFallbackStage` (`handoff.FocusFallbackStage`, values 1..15) and its own
`restoreFallbackMeasured` flag. The result reuses the existing restore fallback enum:
1 success, 2 attach failure, 3 detach failure, 4 API rejection, 5 verification loss.
The stage only refines that result. `FocusFallbackSucceeded` is the sole success stage;
stages 3..8 (terminal-before, window unavailable, thread unavailable, probe failed,
terminal-before-attach, attach failed) are attach, 2 and 11 are detach, 9 and 10 are API
rejection, and 12..15 are verification loss. `Valid` requires the flag and the exact
stage-to-result pairing, rejecting a measured flag with a mismatched pair and an
unmeasured flag with either field set. `ResultMeasured` keeps its existing two bits
unchanged. First write wins on the original source context under the existing
12-attempt/120-second collector bound, and `Retire` deep-copies the fields so later
updates cannot alter an emitted record.

The detailed stage is recorded only by `restoreFocusProfile(ctx)`; legacy activation
keeps `setForegroundWithThreadAttach` and ignores the stage. The shared thread-attach
helper was extracted into `internal/wininput/source_foreground_attach_windows.go` with its
existing call order, probe, ownership checks and outcomes unchanged. Its additional
return value identifies the existing exit. No new native queries, retries, focus policy, wire
or ACL changes, and no raw OS error text, handles or window/thread metadata are retained.

AgentGeneration is 167. The nine-package desktop Go tests and vet with
unsafeptr=false passed with exit 0. Packaging is a separate check. Tests cover legacy
versus profile native call order and counts across eight
scenarios, an actual-owner deactivation probe failure, late/expired/receiver-dropping
input, an invalid stage and export privacy. Device delay and PIN behavior are unverified
on g167.

### g167 device observations

Exports were created at A 2026-10-05T09:59:39Z and B 09:59:37Z, filtered to the exact
revision 6296110a60c59422ea47d14109a336a6cd86fb7b. A has 12 source and B 12
installed-receiver profiles; every handoff ends ok and complete.

All A profiles carry the new fallback diagnostics: states [3,2,2,1] with observed 15,
resultMeasured 3, measured 3 and restoreFallbackMeasured true, with restoreFallback 4
(APIRejected) at restoreFallbackStage 10 (SetFailed), restore 3 (APIRejected) and
postHide 2 (NULL).
The existing code path shows the thread lookup, probe and AttachThreadInput all succeeded
and DetachThreadInput succeeded; SetForegroundWindow exited on error, and the helper
skips ownership verification while attached on that error path. The underlying Windows cause is not yet
known. Every next A attempt begins beforeShow 3 again, so the intervening recovery is
unobserved, and whether the user clicked is still pending.

After the 12-attempt quota, A logged three missing-before-show events at
09:59:13.9985629, 09:59:14.4962803 and 09:59:15.6127829, while profile emission spans
09:58:32.7543292..09:58:43.8789202 (about 11.125s), so no failure profile is retained for
the later events. A request-to-grant is min/median/max 22.804/27.090/57.652ms,
CaptureActivate 5.422/6.0635/23.693ms, SourceDeactivate 4.888/9.2895/14.836ms, restore
2.142/6.365/10.531ms and hide 2.149/3.2935/5.078ms. A's frame histogram is [457,1,0,0,0]
over 458 with a 1.003ms maximum; B's is [47,393,0,0,0] over 440 with a 9.759ms maximum.

B is prepared true on all twelve with ages 2.860..1380.892ms; OpenLock is measured on all
twelve, attempt 1 at 31.507ms exactly the preparation-lock value and all others zero;
ReadyWait is 6.951/7.2695/9.372ms. A CONNECTED at 09:58:30.0735642 and B at
09:58:30.428267, with no later disconnect, reconnect, session stop, connect or heartbeat
failure. B's 12 network timeouts, 2 reconnect attempts, 1 disconnect and 1 session stop
all precede the final connection, the last network timeout at 09:58:30.1596969, and after
CONNECTED B recorded 64 leaseReady, 16 prepareAck and 0 prepareMiss. Both receipts are
result 0/complete, and each export keeps one incomplete historical log with no exact
revision events in it.

PIN and click recovery are not confirmed for g167. Timings are process-local; clock
values from different devices are never subtracted, overlapping spans are not summed,
and these diagnostics make no controlled performance claim. FocusAfterRestore already
observes the foreground freshly before hiding, and there is no evidence yet that an owner
queue pump would help. The next open evidence gap is narrower: on the existing restore
SetForegroundWindow failure path, record the foreground category before detach and
compare it with the post-detach FocusAfterRestore, to separate a failure while the queues
are joined from a transition lost after detach. No policy cause is proved, and no fix or
pump is claimed. The optional user click answer is still pending. This is evidence only,
with no test, build, runtime or API change.

## g168 foreground observation before restore detachment

The g167 failure at SetForegroundWindow leaves the state while input queues are
joined unobserved. g168 adds `restoreRejectedForeground` and
`restoreRejectedForegroundMeasured` to Focus schema 1. The state uses the existing
categories: 1 NULL, 2 capture, 3 saved original, 4 other. Zero is unobserved.
The native capture owner reads GetForegroundWindow once after the attached restore
SetForegroundWindow call reports an error, before starting detach cleanup, only
when the original source profile is live. Activation passes a nil profile. Success,
pre-set terminal errors, probe/attach failures and untracked or retired contexts
make no additional native query.

Compare this category with the same attempt's existing FocusAfterRestore state
(after detachment, before hide), then FocusAfterHide. Observing the saved original
while joined does not override the API rejection or prove that focus will survive
detachment. A later detach failure retains the earlier observation with the existing
detach-failure result and stage. No focus retry, message pump, simulated click,
authority change or native error interpretation is introduced.

Valid requires a measured category in 1..4 and a measured fallback whose stage is
SetFailed or DetachFailed, with the existing result/stage pairing still enforced.
Unmeasured categories must remain zero; legacy g167 profiles remain valid. First
write wins under the existing 12-attempt/120-second source collector bound, and
Retire's value copy freezes both fields. Export retains only numeric categories and
the presence flag; native handles, window metadata and input contents stay absent.

AgentGeneration is 168. Tests cover unchanged legacy native call sequences and
outcomes, exactly one extra observation on the profiled rejection path, its position
before cleanup/detachment, saved-target observation without converting rejection to
success, later detach failure, missing/receiver/retired/ended/quota contexts,
expired recorder input, first-write and snapshot copying, contradictory evidence
and sanitized export. Real-device focus recovery, delay and PIN regression checks
remain pending. Device timings are process-local and overlapping spans are not added.

The nine-package desktop Go tests, vet with unsafeptr=false and diff whitespace
check passed locally. Packaging and bundle verification are separate checks.

## g169 atomic rejection observation

Final review found a collector-retirement gap in g168: recording the optional
category before detach and the fallback outcome after detach could emit a partial
profile if the collector ended between those writes. g169 retains the category in
the owner's local numeric return value until cleanup completes, then records the
category and final result/stage together in one collector update. If the profile
ends during cleanup, neither field is recorded. First write freezes the complete
pair, including an unobserved category, so later cleanup cannot fill it retroactively.
The observation still occurs before detachment; native query counts, call order,
ownership checks and restore outcomes are unchanged from g168. The collector lock
for storing the category now runs after detachment.

A regression test retires the collector from the detach stub and checks that the
emitted summary remains valid with no partial focus evidence, while the original
API rejection and detach cleanup are retained. Packaging did not activate g168;
g169 is the intended device-test bundle. Real-device delay, focus recovery and PIN
behavior remain unverified.

The nine-package desktop Go tests, vet with unsafeptr=false and diff whitespace
check passed locally. Packaging and bundle verification are separate checks.

### g169 device observations

Exports captured for this analysis were created at A 2026-10-05T10:57:15Z and B
10:57:04Z. Both are filtered to revision
ac821e58b094255f66aacb1dca4917d1b9402980. Export files changed during inspection;
the conclusions below use a fixed, ignored analysis snapshot instead of a moving
latest pointer. Earlier exports at A 10:55:10Z and B 10:55:02Z already contain the
same twelve profiles; those earlier snapshots precede the later session failure.

A has twelve source and B twelve installed-receiver profiles. Attempts 3, 4, 5,
11 and 12 end ok; 1, 2 and 6..10 end error. Attempts 1 and 2 have A focus states
[3,3,3,3], skipped restoration, and foreground-probe failure events. Thus these
two activation failures occur with an observable saved foreground, independently
of the later NULL-foreground sequence.

Attempts 3, 4, 5 and 11 have states [3,2,2,1], restore 3 (APIRejected), postHide 2
(NULL), fallback 4 at stage 10 (SetFailed), and the new rejected-foreground category
2 with its measured flag true. Capture therefore remains foreground at the
pre-detach observation and at the post-detach, pre-hide observation. Restoration
was not observed to reach the saved target while queues were joined; these records
do not support a saved-target transition being lost only on detachment. The
underlying Windows reason for rejection remains unobserved.

After attempt 5's failed restore and NULL post-hide observation, attempts 6..10 all
start with NULL and fail activation, recording states [1,1,1,1]. Five
missing-before-show events match these failures. Attempt 11 again begins with the
saved original, but how focus recovered in between is unknown; user clicking has
not been confirmed. Attempt 12 restores successfully with states [3,2,3,3] and
fallback result/stage 1. Its restore span is 202.936ms and source deactivation is
260.360ms. The four rejected restore spans are 37.419/6.747/3.528/4.095ms.

A request-to-grant spans 26.225..171.911ms and capture activation
8.444..1862.742ms. Attempt 9's capture fallback span is 1835.210ms inside its
1862.742ms activation; these overlapping spans are not additive. A's frame histogram
is [363,33,0,1,0] and B's [17,369,11,0,0], each totaling 397 frames. A's worst
dispatch is 544.036ms at attempt 4, matching that vector's InputWrite value; B's
worst dispatch is 17.843ms. These remain separate latency observations, not a
controlled comparison or an attribution of all perceived delay to one cause.

B is prepared true on eleven profiles; attempt 8 has no prepared flag, which is
not interpreted as a preparation duration of zero. ReadyWait spans
6.521..121.449ms, with the largest value at attempt 8. OpenLock and its preparation
overlap are measured on all twelve: attempt 10 is 54.628ms in both, while the
other eleven are measured zero. After B's connection, the fixed snapshot includes
22 lease-ready and 17 preparation-acknowledged events and no preparation-missed or
installed-input connect/heartbeat failure event. Both installation receipts are
result 0/complete; one incomplete historical log per device has no exact-revision
events.

There is no connection loss during the retained profile sequence. A connects at
10:53:56.8264087Z and B at 10:53:57.1168604Z. However, the later snapshot contains a
separate session failure: A records trust-read recovery at 10:56:56.7383439Z,
deferred validation at 10:56:59.7158309Z, and freshness expiry at
10:57:02.7673042Z with the trace active at prelock_directory_acl. A then records
DISCONNECTING at 10:57:03.7483561Z and session_failed at 10:57:03.8824806Z. B
independently records remote EOF and RECONNECTING at 10:57:03.0788222Z. Cross-device
timestamps are not subtracted or used to prove event ordering. The resident trust
monitor's five-second freshness bound cancels a session when validation has not
completed successfully in time. A's eight deferred and seven recovered events
after connection show repeated local read delays; why those filesystem/security
queries stalled is not determined by these coarse trace categories.

The next focus work concerns restore rejection before detachment and the ensuing
NULL foreground, rather than a post-detach completion pump. The later trust
freshness failure needs a separate investigation of validation I/O while preserving
revocation and freshness checks. PIN behavior and the user's click-recovery action
are not confirmed in this export. No runtime, API, database, runner or security
setting was changed. Revision filtering, histogram/frame counts, trace-to-source
mapping and diff whitespace were checked; tests and a rebuild are skipped because
this change records device evidence only.

## g170 guarded queue-local restore

The restore path now makes one SetActiveWindow(savedFocus) request after the
attached SetForegroundWindow restore rejects, while the exact saved target remains
valid, visible, enabled and responsive, and capture still owns foreground.
Activation, NULL/external foreground and terminal runs are excluded. The request
uses the existing attachment, with no extra attach cycle, simulated click or retry
loop. Target foreground must be verified before and after detach; the API return
alone does not prove ownership. Unverified local activation preserves the original
rejection, and detach failure remains terminal. ADR 0030 records the refinement.

Focus schema 1 adds restoreActive and restoreActiveMeasured. Active 1 means target
foreground was observed while joined after this request; 2 means the request was
made but target foreground was not observed. Zero is unobserved/not attempted.
Active 1 can accompany final fallback success, detach failure or post-detach
verification failure; active 2 only accompanies SetFailed or DetachFailed. Both
require measured rejected-foreground category 2. The final fallback still describes
the post-detach outcome. The values are recorded together with that outcome in one
first-write collector update and copied on retirement. No handles, input contents
or window metadata are exported. Legacy g169 profiles remain valid.

The runtime's eligibility queries run independently of the diagnostic budget.
Diagnostic records remain bounded to twelve attempts within 120 seconds. This
changes native call counts only on restore rejection; normal restoration and
capture activation retain their existing policy. It adds one synchronous activation
request in a joined queue and therefore does not establish a latency improvement
or a strict native-call wall-clock bound. The g169 trust-freshness failure is a
separate investigation; trust validation and security settings are unchanged.

Tests cover one request within the existing attachment, success/unverified/detach
failure/post-detach ownership loss/cancellation, invalid or hidden/disabled saved
targets, external and NULL foreground, ownership changes during eligibility,
unchanged activation policy, and a restore-then-reactivate sequence with no
physical click. Schema and masked-export tests preserve first-write evidence and
reject contradictory outcomes. AgentGeneration is 170. Real-device delay, focus
recovery and PIN regression checks remain pending.

The nine-package desktop Go tests, vet with unsafeptr=false and diff whitespace
check passed locally. Packaging and bundle verification are separate checks.

### g170 device observations

Fixed exports were created at A 2026-10-05T11:25:05Z and B 11:25:00Z and filtered
to revision 85d43ac8b6af4dabc23ebd51e667eca715bfc555. A has twelve source profiles
and B twelve installed-receiver profiles; all twelve end ok on both devices.
There is no missing-foreground/probe-failure activation event in these exports.

The added active restore did not demonstrate a successful restoration. A attempts
1..11 all record restoreActive 2 with its measured flag true: the request ran but
target foreground was not observed. Each keeps rejected-foreground category 2,
fallback 4 at stage 10 (SetFailed), restore 3 (APIRejected), postHide 2 (NULL) and
states [3,2,2,1]. Capture remains foreground before and after detach and becomes
NULL after hide. Thus the new runtime branch was exercised eleven times without
changing the restore outcome. Attempt 12 succeeds through the existing fallback
with result/stage 1, restore 2, postHide 1 and states [3,2,3,3]; it has no active
restore measurement, so that success is not attributed to SetActiveWindow.

Each next A attempt nevertheless begins with the saved original observed again.
The mechanism of recovery between NULL after hide and the next attempt remains
unobserved. Whether the user clicked between transfers is pending. Successful
entries therefore do not establish that the added branch resolved click recovery.
Its returned previous-window value/error is not recorded, so unverified target
foreground does not identify why the native activation failed to take effect.

A request-to-grant is min/median/max 23.841/28.937/277.322ms, capture activation
5.507/7.7225/41.626ms, and source deactivation 4.809/6.656/71.349ms. Attempt 12's
successful restore spans 68.401ms. A's frame histogram is [657,3,0,0,0] over 660
frames with a 4.499ms worst dispatch. B's is [68,568,2,0,0] over 638 frames with a
12.817ms worst dispatch. Each histogram was checked against its own frame count;
cross-device totals are not asserted equal or interpreted as packet loss. Spans
overlap and are not summed; this is not a controlled performance comparison.

B is prepared true on all twelve with ages 2.975..1959.380ms. ReadyWait spans
6.552..9.057ms. OpenLock and its preparation overlap are measured on all twelve:
attempt 1 records 73.383ms in both, and the other eleven record measured zero.
After CONNECTED the export includes 21 lease-ready and 8 preparation-acknowledged
events, no preparation-missed or installed-input connect/heartbeat failure event.

A CONNECTED at 11:24:20.6836289Z and B at 11:24:20.9492944Z. There is no subsequent
disconnect, reconnect, session failure or trust-freshness expiry through these
export endpoints, roughly 44 seconds on A and 39 seconds on B. B's one network
timeout and RECONNECTING precede CONNECTED. This short interval does not assess
the longer-running trust-validation failure observed in g169. No emitted slow
trust trace is not a zero-duration measurement. Both installation receipts are
result 0/complete, and each retains one incomplete historical log with no current
revision events. PIN behavior and perceived delay are not confirmed by this export.

The next focus decision must reconsider the ineffective active-restore branch
and the unresolved recovery between transfers; repeating successful entry tests
alone cannot validate restoration. No new runtime, API, database, runner or
security changes are made in this evidence update. Revision filtering,
active-result/state consistency, per-profile histogram counts, source mapping and
diff whitespace were checked. Tests and rebuild are skipped for documentation
only; g170 remains the current packaged generation.

## g171 retire unverified active restoration

Remove the g170 cross-thread SetActiveWindow(savedFocus) request after eleven
device attempts returned restoreActive 2 without verified saved-target ownership.
Keep the existing attach/detach verification and degraded release policy. The
historical focus schema remains readable; new runs do not emit restoreActive.
Rejection observation again requires a live source profile and is committed
atomically with its fallback result. No device improvement is claimed.

### Own-queue preparation for missing foreground

The transition wait now makes at most one SetActiveWindow(capture) request only
after a fresh NULL observation, after pending-message dispatch, with no pending
queue attachment. It services sent messages and checks the actual owner again;
an existing owner adds no preparation cost. Remaining NULL still follows the
existing five-attempt foreground request/poll budget. No new timers, physical
clicks, external-window activation or attachment cycles are introduced.

Hosted fake-native tests cover own-target preparation, external-owner arrival,
one-shot refusal, cancellation during preparation/dispatch, existing owners and
pending-attachment exclusion. CaptureForeground/CaptureFallback timings retain
their existing boundaries. These tests cannot establish native success, improved
device latency or the mechanism of recovery between real transitions. Compare
fresh g171 source profiles with the same-generation receiver observations; keep
cross-host clocks and overlapping spans separate.

### g171 device observations

The exports created on 2026-10-05 at 11:56:09Z (A) and 11:56:05Z (B) contain
revision 7012b338a7d0adc86fcbe69bc4e07d081c9ffdd4. Filter by that exact revision:
A has twelve source profiles and B twelve installed-receiver profiles, each
ending ok and incomplete false. These are not twenty-four independent successful
transfers or a complete account of the trial: the detailed collector stops after
twelve attempts (within its 120-second window).

All twelve A profiles record focus states [3,2,2,1], restore 3 (APIRejected),
postHide 2 (NULL), fallback 4 at stage 10 (SetFailed) and rejected foreground 2
(capture). No restoreActive measurement is emitted. Thus every profiled release
retains capture at the rejected restore, then has no foreground after hide.
Later successful attempts start with saved-original foreground again, but the
recovery mechanism between attempts is unobserved.

After A's twelfth profile at 11:55:10.9829452Z, six separate
activation_capture_foreground_missing_before_show events are recorded between
11:55:44.2501306Z and 11:55:54.4188333Z. They identify failed activation attempts
that started with NULL foreground. Their detailed focus/capture profiles are
absent after the quota, so the new own-queue preparation's execution and result
cannot be independently attributed. g171 has not eliminated this failure class.
The operator reports a blocked crossing followed by successful movement without
a click. The precise relation to these events and the native recovery mechanism
remain unobserved; do not describe eventual recovery as a complete fix.

For the twelve successful source profiles, RequestToGrant min/median/max is
22.424/27.659/34.658ms, CaptureActivate 5.962/8.117/25.849ms and SourceDeactivate
4.877/12.9145/20.307ms. CaptureFallback is 3.777/4.7375/10.818ms. These local,
possibly nested spans must not be added. Failed attempts outside the quota are
not represented in these distributions. This is an observation, not a controlled
comparison with g170 or evidence of pointer presentation latency.

A's histogram totals [529,2,0,0,0] over 531 frames; its worst Dispatch is
1.509ms. B totals [34,483,2,0,0] over 519 frames and worst Dispatch is 28.012ms.
Each profile's histogram sum equals its own frame count. B is prepared true on
all twelve, ReadyWait is 6.472..8.178ms, and OpenLock is measured zero on all
twelve. B records sixty lease-ready and nineteen preparation acknowledgements,
with no installed-input connection or heartbeat failure event in this revision.

A CONNECTED at 11:54:55.3315166Z and B at 11:54:55.5625859Z. No subsequent
disconnect, reconnect, session failure or trust expiry is recorded through their
respective exports, about 74 seconds on A and 69 seconds on B. B's two acquisition
timeouts and RECONNECTING occur before CONNECTED. The six local activation
failures are distinct from connection failure. Both installation receipts are
result 0/complete, and each export has one incomplete historical log containing
no current-revision events. PIN behavior is unconfirmed by this export.

The next focus work must address the rejected saved-window restore and subsequent
NULL foreground, preserving degraded release and terminal detach/retained-capture
guards. Repeating activation retries alone is not supported as a remedy. This
update records evidence only: no runtime, API, database, runner or security change.
Checks cover revision isolation, event identity uniqueness, histogram conservation,
measured-span masks, focus categories and same-host event ordering. Tests and
rebuild are skipped for documentation-only changes; g171 remains the packaged
generation.

## g172 post-hide restoration

After the original restore rejects, a fully active capture with successful
cleanup now attempts the saved window once after hiding, only while actual
foreground is NULL. Target validity, visibility, enabled state, responsiveness,
terminal state and released registration/attachment are rechecked before the
request. Intervening external owners stop this path. Original restore rejection
and fallback categories are retained; final post-hide release guards still run.

Optional focus fields postHideRestoreMeasured/Accepted/Foreground/Micros separate
API acceptance from actual ownership and record one local duration, including
the pre-request probe and possible completion probe. The foreground categories
match existing focus states. This work occurs after the original restore/hide
spans; SourceDeactivate includes it and must not be added to those nested spans.
At most two existing 100ms responsiveness probes and one foreground request are
added on this guarded failure path. No sleeping retry loop or attachment cycle
is added. Actual native-call latency and improvement remain unmeasured.

Hosted tests cover repeated returns after pre-hide rejection, accepted-but-NULL
and rejected requests, asynchronous completion, intervening owners, disabled or
destroyed targets, pending attachment, cancellation, retained hidden capture,
degraded release, frozen attempts and sanitized numeric export. A fresh device
trial must check post-hide saved-target ownership, later activation failures,
crossings without physical clicks and PIN behavior before declaring resolution.
