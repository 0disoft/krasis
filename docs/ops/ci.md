# CI

- 상태: Experimental
- provider: GitHub Actions
- workflow: `.github/workflows/ci.yml`

## 실행 경계

`main` push와 모든 pull request에서 Windows job 하나를 실행한다. job은 repository contents read 권한만 가지며 network service, secret, release와 deployment를 사용하지 않는다.

순서는 format 검사, `go test ./...`, `go vet`, Windows CLI build와 장비 실행 kit 생성이다. Win32 callback의 `LPARAM` 변환 package만 `unsafeptr` 검사를 제외하고 나머지 vet analyzer는 유지한다. 성공한 build는 실행 파일, 수집·순차 Phase 0·무결성 helper, 장비 runbook과 build identity를 zip 하나로 묶고 각 파일의 SHA-256 manifest와 commit SHA가 포함된 artifact 이름으로 올린다.

peer 통합 테스트는 먼저 한 process 안의 두 `RunSession`을 실제 framed stream으로 연결해 request, grant, input, return revoke와 양쪽 `LOCAL` 수렴을 검증한다. 별도 subprocess 테스트는 임시 test CA와 identity를 test temp directory에만 생성하고 source와 destination test process를 localhost TCP로 연결한다. 양쪽은 제품 `PeerConnection`을 사용해 TLS 1.3 mutual authentication과 `krasis/1` ALPN을 통과한 뒤 동일한 왕복을 수행해야 하며 두 process가 모두 success exit code를 반환해야 한다. test certificate와 private key는 artifact에 포함하지 않는다.

semantic write coordinator 테스트는 operation 직렬화, 취소된 waiter 미실행, heartbeat/revoke
순서, revoke 뒤 heartbeat 폐기, connection-close-before-cleanup과 concurrent source activation
뒤 suppression 재해제를 검증한다. lifecycle session 테스트는 malformed input과 heartbeat가
coordinator 밖에서 runtime을 바꾸지 않고 malformed control이 connection close 뒤 cleanup되는지
검증한다. heartbeat timer와 실제 congestion timeout은 아직 CI 범위가 아니다.

optional liveness worker 테스트는 잘못된 interval/timeout 관계를 거부하고, inactive runtime에서
frame을 만들지 않으며, active session heartbeat를 기록하고, silent peer deadline과 heartbeat
write failure에서 runtime을 `LOCAL`로 cleanup하며, deadline의 held input release와 blocked
write 취소 뒤 coordinator 재사용, coordinator 대기 뒤 fresh deadline observation, deadline과
session cancellation 경합에서 bounded held-input cleanup을 검증한다. observer 테스트는 성공한
송수신만 집계하고 session 경계를 넘는 gap을 만들지 않는지 검증한다. lifecycle receive 테스트는
coordinator 획득 뒤 deadline에 도달한 heartbeat가 `LastObserved`를 갱신하거나 observer에
기록되지 않고 held input을 해제하는지 검증한다. 실제
timer cadence, congestion과 절전 중 false timeout은 장비 실험 범위다.

agent assembly 통합 테스트는 두 agent session을 실제 framed pipe로 연결하고 source worker ready, boundary intent, lease grant 이후 suppression, normalized input 전달, destination return intent, source deactivation과 양쪽 `LOCAL` cleanup을 한 경로에서 검증한다. 테스트는 OS hook과 `SendInput` 대신 같은 interface의 deterministic port를 사용하므로 실제 Windows callback과 주입 검증을 대체하지 않는다.
별도 component lifecycle 테스트는 session context가 살아 있는 동안 worker가 독립적인
`context.Canceled` 또는 `context.DeadlineExceeded`를 반환하면 전체 session이 즉시 취소되고
`LOCAL`로 끝나는지 검증한다.
부모 cancellation에서 파생된 worker 종료만 기존 teardown cause를 보존해야 한다.

supervisor 테스트는 명시적인 transition table, exponential backoff cap, acquire failure 뒤 retry,
`LOCAL` session failure 뒤 reconnect와 non-local failure의 즉시 중단을 검증한다. terminal path는
예상 밖의 clean session return과 unsafe final state에서도 source와 connection을 닫고 마지막
snapshot을 `STOPPED`로 기록해야 한다. acquire error와 함께 반환된 부분 connection도 retry 전에
닫히는지 검증한다. network source 통합 테스트는 `peer-credentials`가 만든 역할 분리 identity로 실제 localhost listener/dial을 열고 양쪽 `PeerConnection`의 mTLS와 ALPN 인증을 통과시킨다.

emergency release 테스트는 modifier press/release, left/right modifier variant, F12 terminator 비전달, source suppression 해제와 worker fault cleanup을 검증한다. `agent-session` parser와 evidence gate 테스트는 explicit live-input acknowledgement, bounded duration, 인증 session 부재와 destination input 부재를 성공으로 가장하지 않는지 확인한다. CI에서는 실제 hook suppression과 `SendInput` 상호작용을 실행하지 않는다.

crash guard protocol 테스트는 down 전 prepare ACK, up 후 release ACK, 역순 cleanup, malformed
frame과 release 실패를 검증한다. 별도 subprocess 테스트는 guard에 token을 준비한 parent
test process의 readiness 신호를 받은 뒤 parent만 강제 종료하고, pipe EOF를 받은 guard가
독립적으로 recovery marker를 기록하는지 bounded polling으로 확인한다. marker에는 test용
normalized kind와 code만 있으며 artifact나 제품 report로 보존하지 않는다.
별도 hung-helper 테스트는 command를 읽고 ACK하지 않는 guard를 사용해 2초 제품 timeout을
짧은 test timeout으로 대체하고, parent가 IPC 실패를 반환하며 helper를 종료하는지 확인한다.
guard child 테스트는 정상 disarm ACK 뒤 process가 종료되기 전에 strict JSON evidence를
no-replace로 쓰는지 검증한다. guard report checker는 parent EOF에서 실제 release가 없거나,
dirty/mismatched build, unknown field, invalid count와 정상 disarm recovery를 통과시키지 않는다.

agent report checker 테스트는 strict JSON decoding과 두 file cardinality, 장소별 alias pair,
listener/connect 역할, 반대 edge, clean 동일 build, 동일 duration/heartbeat interval/peer timeout,
최소 heartbeat 송수신·same-session gap evidence와 timeout보다 작은 최대 gap, 양쪽 pass, crash guard의
시작·정상 종료, 같은 directory의 guard report device/build/zero-recovery 및 destination evidence를 검증한다. 전체 path, session identifier나 원시
입력은 checker output으로 복제하지 않는다.

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
