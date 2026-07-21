# CI

- 상태: Experimental
- provider: GitHub Actions
- workflow: `.github/workflows/ci.yml`

## 실행 경계

`main` push와 모든 pull request에서 Windows job 하나를 실행한다. job은 repository contents read 권한만 가지며 network service, secret, release와 deployment를 사용하지 않는다.

순서는 format 검사, `go test ./...`, `go vet`, Windows CLI build와 장비 실행 kit 생성이다. `internal/wininput`은 Win32 callback `LPARAM`, message structure와 syscall buffer를 Go pointer로 연결하므로 이 package에 한해 `unsafeptr` analyzer를 제외한다. 해당 package의 나머지 vet analyzer와 다른 모든 package의 기본 vet analyzer는 유지한다. 이 예외는 memory safety 증명이 아니며 새 `unsafe` 경계는 Win32 ABI lifetime, struct layout과 callback 범위를 별도로 검토해야 한다. 성공한 build는 실행 파일, 수집·순차 Phase 0·무결성 helper, 장비 runbook과 build identity를 zip 하나로 묶고 각 파일의 SHA-256 manifest와 commit SHA가 포함된 artifact 이름으로 올린다.

peer 통합 테스트는 먼저 한 process 안의 두 `RunSession`을 실제 framed stream으로 연결해 request, grant, input, return revoke와 양쪽 `LOCAL` 수렴을 검증한다. 별도 subprocess 테스트는 임시 test CA와 identity를 test temp directory에만 생성하고 source와 destination test process를 localhost TCP로 연결한다. 양쪽은 제품 `PeerConnection`을 사용해 TLS 1.3 mutual authentication과 `krasis/1` ALPN을 통과한 뒤 동일한 왕복을 수행해야 하며 두 process가 모두 success exit code를 반환해야 한다. test certificate와 private key는 artifact에 포함하지 않는다.

semantic write coordinator 테스트는 operation 직렬화, 취소된 waiter 미실행, heartbeat/revoke
순서, revoke 뒤 heartbeat 폐기, connection-close-before-cleanup과 concurrent source activation
뒤 suppression 재해제를 검증한다. lifecycle session 테스트는 malformed input과 heartbeat가
coordinator 밖에서 runtime을 바꾸지 않고 malformed control이 connection close 뒤 cleanup되는지
검증한다. peer session close-owner 테스트는 EOF와 context cancellation에서 connection을 한 번만
닫고 원래 종료 원인, Close 실패와 `LOCAL` cleanup 결과를 함께 보존하는지 검증한다. heartbeat
timer와 실제 congestion timeout은 아직 CI 범위가 아니다.

optional liveness worker 테스트는 잘못된 interval/timeout 관계를 거부하고, inactive runtime에서
frame을 만들지 않으며, active session heartbeat를 기록하고, silent peer deadline과 heartbeat
write failure에서 runtime을 `LOCAL`로 cleanup하며, deadline의 held input release와 blocked
write 취소 뒤 coordinator 재사용, coordinator 대기 뒤 fresh deadline observation, deadline과
session cancellation 경합에서 bounded held-input cleanup을 검증한다. observer 테스트는 성공한
송수신만 집계하고 session 경계를 넘는 gap을 만들지 않는지 검증한다. lifecycle receive 테스트는
coordinator 획득 뒤 deadline에 도달한 heartbeat가 `LastObserved`를 갱신하거나 observer에
기록되지 않고 held input을 해제하는지 검증한다. 실제
timer cadence, congestion과 절전 중 false timeout은 장비 실험 범위다.
별도 cleanup timeout 테스트는 coordinator가 제한 시간보다 오래 점유됐을 때 operation이 실행되지
않고 runtime이 기존 non-local state를 유지하며, 반환 오류가 원래 session fault와
`context.DeadlineExceeded`를 모두 포함하는지 검증한다.
cleanup recovery 테스트는 첫 `ReleaseAll`에서 실패한 token만 한 번 더 시도해 transient 실패는
`LOCAL`로 수렴하는지 검증한다. 두 시도 모두 실패하는 경우 정확히 두 번만 release를 호출하고
`FAULT`와 pressed token을 유지해야 한다.

agent assembly 통합 테스트는 두 agent session을 실제 framed pipe로 연결하고 source worker ready, boundary intent, lease grant 이후 suppression, normalized input 전달, destination return intent, source deactivation과 양쪽 `LOCAL` cleanup을 한 경로에서 검증한다. 테스트는 OS hook과 `SendInput` 대신 같은 interface의 deterministic port를 사용하므로 실제 Windows callback과 주입 검증을 대체하지 않는다.
runtime 테스트는 local return 직후 exact retired session·epoch의 연속 input을 주입 없이 drain하고 다른
session을 거부하는지 검증한다. supervisor 테스트는 session error가 이미 소유한 connection Close 오류만
retry하며 독립적인 Close 실패는 terminal로 유지하는지 검증한다.
별도 component lifecycle 테스트는 session context가 살아 있는 동안 worker가 독립적인
`context.Canceled` 또는 `context.DeadlineExceeded`를 반환하면 전체 session이 즉시 취소되고
`LOCAL`로 끝나는지 검증한다.
부모 cancellation에서 파생된 worker 종료만 기존 teardown cause를 보존해야 한다.

supervisor 테스트는 명시적인 transition table, exponential backoff cap, acquire failure 뒤 retry,
`LOCAL` session failure 뒤 reconnect와 non-local failure의 즉시 중단을 검증한다. terminal path는
예상 밖의 clean session return과 unsafe final state에서도 source와 connection을 닫고 마지막
snapshot을 `STOPPED`로 기록해야 한다. acquire error와 함께 반환된 부분 connection도 retry 전에
닫히는지 검증한다. session과 supervisor의 중복 Close는 실제 transport에 한 번만 전달되고,
connection 또는 source Close 실패는 원래 session·shutdown 오류와 함께 반환되어야 한다.
network source 테스트는 listener의 첫 Close 오류 보존과 TLS 인증 실패 뒤 raw socket의 close
오류 결합을 검증한다. 통합 테스트는 `peer-credentials`가 만든 역할 분리 identity로 실제 localhost listener/dial을 열고 양쪽 `PeerConnection`의 mTLS와 ALPN 인증을 통과시킨다. trust store 테스트는 import/list/revoke persistence, corrupt·unknown·trailing·oversized JSON 거부, duplicate와 repeated revoke, permissive directory, process lock contention, atomic replace revision과 temporary cleanup을 검증한다.

emergency release 테스트는 modifier press/release, left/right modifier variant, F12 terminator 비전달, source suppression 해제와 worker fault cleanup을 검증한다. `agent-session` parser와 evidence gate 테스트는 explicit live-input acknowledgement, bounded duration, 인증 session 부재, destination input 부재, 주입 실패와 cursor rollback 실패를 성공으로 가장하지 않는지 확인한다. CI에서는 실제 hook suppression과 `SendInput` 상호작용을 실행하지 않는다.

`pointer-shadow` unit test는 legacy hook 좌표가 edge에 고정되면 outward X delta가 사라지는 characterization, Raw Input relative/absolute packet 분류, Precision Touchpad의 zero device handle aggregate와 JSON privacy shape를 검증한다. hosted CI는 실제 mouse를 edge에서 계속 움직이거나 Raw Input과 hook의 runtime ordering을 재현하지 않으므로 장비별 shadow report가 별도 필요하다.

`wheel-surface` unit test는 physical·합성 wheel 분류, Raw Input vertical/horizontal signed delta,
zero-device·tagged-injected 집계, 네 observed path와 privacy shape를 검증한다. hook health sentinel은
wheel이 아니라 tagged `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE` 상대 `+1/-1` pair이며 tagged
`WM_MOUSEMOVE`만 삼켜 Raw wheel evidence를 오염시키지 않아야 한다. CLI test는 sentinel health와
non-sentinel physical move/button interaction을 pass gate로 검증한다. hosted CI는 gram13 Precision
Touchpad의 실제 입력 표면이나 hook 누락 시 cursor 순간 이동 위험을 증명하지 못하므로 단일 장비
report가 필요하다.

`touchpad-surface` unit test는 pointer message 종류별 aggregate, `PT_TOUCHPAD`/other/query failure
분류, legacy-only 실패, foreground loss, API unsupported, no-overwrite와 privacy shape를 검증한다.
hosted CI는 visible window의 실제 foreground 획득, Windows 11 touchpad capability 등록 결과나 gram13
Precision Touchpad의 두 손가락 gesture를 재현하지 않는다. 따라서 실제 장비 report가 별도 필요하다.

source capture unit test는 window legacy wheel의 signed axis mapping, 한 message당 한 queue action,
inactive source fault, low-level hook의 physical·untagged pass-through와 tagged suppression, combined source/capture
activation·rollback·deactivation 순서, bounded command admission/result timeout, capture/focus evidence 합계와
startup readiness gate를 검증한다. v18은 정상 foreground 경로 우선, 첫 실패에만 thread-input attach,
단일 retry, 성공·실패 뒤 detach, detach 실패의 terminal rollback과 fallback outcome 합계를 추가로 검증한다.
	v19는 saved-focus 복원에도 정상 경로 우선, 단일 attach/retry/detach와 최종 restored/failed 합계 보존을
	대칭 검증한다.
	v20은 정확한 restore 실패 뒤 capture window를 숨기고 post-hide foreground release를 분류하며,
	recovered·unavailable은 non-terminal, capture-retained와 thread detach 실패는 terminal인지 검증한다.
hosted CI는 `SetForegroundWindow`, `AttachThreadInput`, 작은 edge window의 실제 touchpad
delivery, `WM_ACTIVATE` ordering과 이전 foreground application 복원을 사용자 desktop에서 재현하지 않으므로
두 장비 수동 gate가 필요하다.

production mouse source unit test는 8-byte buffer alignment, variable-size Raw Input block traversal과
truncation 거부, zero-device relative move, absolute·known tagged move 배제, 한 packet의
move·button·signed wheel 순서, active/inactive/injected hook suppression과 pointer action 중복 부재를
검증한다. hosted CI는 `GetRawInputData`와 `GetRawInputBuffer`의 실제 queue ordering, hook suppression
뒤 Raw Input 도착, 1000Hz queue 여유와 두 장비의 체감 pointer 품질을 증명하지 않는다.

Windows lifecycle 테스트는 숨겨진 top-level window monitor의 start/close, power·WTS session·shutdown·
display message 정규화, 첫 event cancellation, monitor 비정상 종료, guard 선행 종료와 source-before-guard
close-once 순서를 검증한다. hosted runner는 실제 Modern Standby, 잠금 화면과 display 재구성을
발생시키지 않으므로 message 수신부터 실제 input cleanup까지의 end-to-end 증거는 장비 실험 범위다.

crash guard protocol 테스트는 down 전 prepare ACK, 중복 prepare 구분, 확실한 무주입의 cancel ACK,
up 후 release ACK, 역순 cleanup, malformed
frame과 release 실패를 검증한다. 별도 subprocess 테스트는 guard에 token을 준비한 parent
test process의 readiness 신호를 받은 뒤 parent만 강제 종료하고, pipe EOF를 받은 guard가
독립적으로 recovery marker를 기록하는지 bounded polling으로 확인한다. marker에는 test용
normalized kind와 code만 있으며 artifact나 제품 report로 보존하지 않는다.
별도 hung-helper 테스트는 command를 읽고 ACK하지 않는 guard를 사용해 2초 제품 timeout을
짧은 test timeout으로 대체하고, parent가 IPC 실패를 반환하며 helper를 종료하는지 확인한다.
guard child 테스트는 정상 disarm ACK 뒤 process가 종료되기 전에 strict JSON evidence를
no-replace로 쓰는지 검증한다. guard report checker는 parent EOF에서 실제 release가 없거나,
dirty/mismatched build, unknown field, invalid count와 정상 disarm recovery를 통과시키지 않는다.

cursor entry 보상 테스트는 prepare 뒤 inbound grant 또는 grant frame write 실패에서 rollback이
호출되는지, 성공한 grant만 commit되는지, 현재 cursor가 entry 좌표에서 이미 움직였으면 snapshot을
덮어쓰지 않는지 검증한다. Windows API mock은 순서를 검증하지만 실제 물리 mouse와의 경쟁을 증명하지 않는다.

agent report checker 테스트는 strict JSON decoding과 두 file cardinality, 장소별 alias pair,
listener/connect 역할, 반대 edge, clean 동일 build, 동일 duration/heartbeat interval/peer timeout,
최소 heartbeat 송수신·same-session gap evidence와 timeout보다 작은 최대 gap, 양쪽 pass, crash guard의
시작·정상 종료, lifecycle monitor의 clean 종료와 event·rearm 부재, 같은 directory의 guard report device/build/zero-recovery, 서로 보완되는 source/destination evidence, 주입 실패 0건과 cursor rollback 분류 합계를 검증한다. control-flow 단계 보존, stage failure·not-ready drop 거부, structured failure count 합과 stable triple도 검증한다. source/destination action, 주입 실패, cursor rollback, control-flow와 failure count의 `uint64` overflow는 별도 실패 증거로 검증한다. 전체 path, session identifier나 원시
입력은 checker output으로 복제하지 않는다.

Windows `cmd/krasis-lab` 통합 smoke는 현재 source에서 임시 `krasis-lab.exe`를 실제로 빌드해
subprocess로 실행한다. 정상 `agent-session-check/v18`, physical·합성·active Raw·window wheel source pipeline과 capture/focus lifecycle·foreground/focus-restore fallback outcome·post-hide focus release·source queue 체류·destination disposition·거절 사유 합계·focus 복원 실패 원인 합계, local interaction admission, commit/abort control-flow와 structured failure evidence,
주입 실패, cursor rollback 합계 불일치와 실패, 정상 `phase0-check/v17`를 검증하고 모든 JSON output을 재귀 검사해 token code, pointer 좌표·delta,
session·epoch·lease, 주소와 certificate 식별 필드가 나타나지 않는지 확인한다.

control-flow unit test는 boundary admission 네 분류, local menu가 identity 생성과 request 전에 차단되는지,
semantic request/grant/entry write 순서, source와
capture activation 단계, local connection interrupt cause가 뒤따른 peer read 오류에 가려지지 않는지
검증한다. 이 aggregate test는 세대별 trace나 실제 Wi-Fi ordering을 증명하지 않는다.

source session 단위 테스트는 blocking `Interrupt`가 cleanup deadline을 넘겨 parent를 멈추지 않는지,
source input thread join은 hung child를 bounded `thread-join` failure로 분류하는지 검증한다. return boundary
테스트는 rearm distance가 cursor entry inset 이하이거나 현재 화면 폭 이상이면 거부하는지 검증한다.
capture window lifecycle 테스트는 foreground 획득 전에 실패한 activation cleanup의 focus 결과가 성공한
activation/deactivation evidence에 섞이지 않는지 검증한다. fallback 테스트는 attach/detach 대상 ID 자체를
report에 저장하지 않고 aggregate outcome만 증가시키는지도 구조 검증한다.

Phase 0 checker 테스트는 서로 다른 날짜의 home/study pair를 같은 build, duration과 liveness policy로
합칠 수 있는지, 네 alias의 parent-EOF recovery가 각각 한 번씩 있는지 검증한다. 네 장비의
동시 실행이나 같은 수집 timestamp를 요구하지 않으며 location 사이 policy가 다르면 거부한다.

subprocess fault 테스트는 active session에서 destination process를 강제 종료한 뒤 source process가 connection failure를 관찰해 suppression을 해제하고 `LOCAL`로 성공 종료하는지 확인한다. 반대 방향은 destination에 실제 key-down을 전달한 뒤 source process를 강제 종료하고 destination이 해당 key release를 adapter에 실행한 다음 `LOCAL`로 성공 종료하는지 확인한다. 부모 test가 명시적으로 종료한 process의 non-zero exit만 예상 결과로 허용한다.

재연결 통합 테스트는 같은 source와 destination runtime이 첫 session을 `LOCAL`로 종료한 뒤 새 session ID와 epoch로 두 번째 request, grant, input과 revoke 왕복을 완료하는지 확인한다. 별도 stale-input 테스트는 두 번째 active session의 framed stream에 첫 session의 envelope를 주입하고 destination input port가 해당 action을 실행하지 않으며 양쪽 session goroutine의 fault cleanup이 끝난 뒤 runtime이 fail-safe `LOCAL`로 수렴하는지 확인한다.

checkout, Go setup과 artifact upload action은 tag가 아니라 검토한 commit SHA로 고정한다. Go version은 `go.mod`의 language floor를 사용하고 dependency가 없는 현재 module에서는 setup action cache를 끈다.

## CI가 증명하지 않는 것

hosted runner는 실제 사용자의 interactive desktop 입력 장치와 네 대상 노트북을 대표하지 않는다. subprocess peer test도 localhost의 정상 연결만 검증하며 공용 Wi-Fi, 방화벽, packet loss, 절전, process 강제 종료와 실제 두 장비의 지연을 증명하지 않는다. 따라서 `diagnose`, low-level hook `capture`, `SendInput` selftest와 `probe`는 CI에서 실행하지 않는다. 이 동작은 대상 Windows session에서 별도 smoke와 장비별 report로 검증한다.

## 실패 정책

- format, test, vet 또는 build 중 하나라도 실패하면 artifact를 만들지 않는다.
- artifact는 배포물이나 서명된 release가 아니라 동일 commit을 수동 장비 실험에 전달하기 위한 experimental probe kit다.
- branch protection의 required check 지정 여부는 repository 설정에서 아직 `UNDECIDED`다.

## 로컬 대응

로컬 검증은 mustflow의 `krasis_go_format`, `krasis_go_test`, `krasis_go_vet`, `krasis_go_build_lab` intent가 소유한다. 실제 입력 smoke는 `krasis_lab_diagnose_smoke`, `krasis_lab_capture_smoke`, `krasis_lab_selftest_inject_smoke`, `krasis_lab_stress_selftest_smoke`, `krasis_lab_probe_smoke`, `krasis_lab_guided_probe_smoke`로 분리한다. package parity는 `krasis_probe_kit_package_smoke`가 checksum과 build metadata까지 확인한다.

## 남은 위험

GitHub가 workflow를 실제로 수락하고 `windows-latest` image에서 성공하는지는 push 후 첫 run으로 확인해야 한다. hosted image와 action runtime 변경도 commit SHA pin만으로 완전히 제거되지는 않는다.
