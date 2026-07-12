# 장비별 probe 실행

- 상태: Experimental
- 대상: `H-A`, `H-B`, `S-A`, `S-B`
- 실행 방식: 사용자 수동 실행 전용

## 전제

GitHub Actions의 `krasis-probe-kit-windows-amd64-<commit>.zip` artifact를 한 번 내려받아 두 장소에서 같은 zip을 사용한다. 이 artifact는 서명된 release나 설치 프로그램이 아니다. Windows가 출처 경고를 표시할 수 있으며, commit과 checksum을 확인할 수 없는 복사본은 실행하지 않는다.

네 대를 동시에 켤 필요가 없다. 집에서 `H-A`, `H-B`를 각각 실행하고 다른 날 스터디카페에서 `S-A`, `S-B`를 실행해도 된다. 각 장비에서는 다른 프로그램의 중요한 작업을 저장하고 마우스나 키보드를 자동 조작하는 프로그램을 잠시 끈다.

## 무결성 확인

새 directory에 압축을 푼 직후 실행 파일, helper, runbook과 build metadata 전체의 SHA-256을
확인한다. 검증기는 manifest에 없는 추가 파일이나 하위 directory도 거부한다. 아래 PowerShell은
사람의 수동 확인용이며 agent command 권한을 부여하지 않는다.

```powershell
.\verify-kit.ps1
```

`Probe kit checksums verified.`가 나오지 않으면 실행하지 않는다. 이 검사는 다운로드 뒤
부분 변조와 손상을 찾지만, artifact와 `SHA256SUMS.txt`가 함께 교체된 경우를 막는 서명은
아니다. GitHub workflow run, commit과 artifact 출처도 함께 확인한다.

## 수집

장비 별칭을 실제 장소와 노트북에 맞게 하나만 선택한다. 아래 명령은 사람의 수동 실행용이다.

```powershell
.\collect-probe.ps1 -DeviceAlias H-A
```

기본 `Basic` mode의 capture 구간은 30초다. 안내가 나오면 그 구간에 pointer 이동, wheel, 기본 mouse button과 keyboard를 각각 사용한다. script는 `reports` directory 아래에 UTC timestamp가 포함된 새 JSON을 만들며 기존 보고서를 덮어쓰지 않는다.

입력 종류별 누락을 바로 확인하려면 `Guided` mode를 사용한다. keyboard, pointer, wheel/buttons 세 단계가 각각 기본 10초 동안 실행된다.

```powershell
.\collect-probe.ps1 -DeviceAlias H-A -Mode Guided
```

다른 장비에서는 별칭만 바꾼다. 집은 `H-A`, `H-B`, 스터디카페는 `S-A`, `S-B`다. 같은 노트북에 다른 별칭을 재사용하지 않는다.

`Basic` mode 보고서를 모은 뒤 아래 명령으로 schema와 비교 조건을 확인할 수 있다. 이 명령도 사람의 수동 실행용이다.

```powershell
$reports = @(Get-ChildItem -LiteralPath .\reports -Filter '*-basic-*.json' -File | Select-Object -ExpandProperty FullName)
.\krasis-lab.exe probe-check @reports
```

서로 다른 build revision, dirty build, duration 또는 중복 장비 별칭이 섞이면 비교 불가능으로 판정한다. `Guided` report는 현재 `probe-check` 입력이 아니며 stage별 `passed`와 `failureCodes`를 직접 확인한다.

## 두 장비 peer probe

네 대를 동시에 켤 필요는 없다. 집에서는 `H-A`와 `H-B`, 스터디카페에서는 별도 날짜에 `S-A`와 `S-B`로 같은 절차를 반복한다.

먼저 test 전용 credential bundle을 만든다. `192.0.2.10`은 문서용 예시이므로 listener의 실제 LAN IP 또는 DNS name으로 바꾼다. 생성 directory는 기존에 존재하면 안 된다.

```powershell
.\krasis-lab.exe peer-credentials --output .\secrets\home-pair --server-name 192.0.2.10
```

`listener/`만 listener 노트북으로, `connector/`만 connector 노트북으로 안전하게 복사한다. 각 directory의 `peer.pem`은 상대 역할의 공개 인증서이며 allowlist fingerprint source다. bundle 전체와 상대 역할의 private key를 두 장비에 복제하지 않는다. key는 Git, report 폴더 또는 동기화 드라이브에 넣지 않고 24시간 안에 실험을 마친 뒤 삭제한다.

listener에서 먼저 실행한다.

```powershell
.\krasis-lab.exe peer-probe listen --device H-A --address 0.0.0.0:24876 --ca .\secrets\listener\ca.pem --cert .\secrets\listener\identity.pem --key .\secrets\listener\identity-key.pem --output .\reports\H-A-peer.json
```

Windows 방화벽과 같은 LAN에서 listener의 실제 IP와 port 접근을 허용한 뒤 connector에서 실행한다. 아래 주소와 server name은 문서용 예시이므로 실제 listener 주소와 certificate name으로 바꾼다.

```powershell
.\krasis-lab.exe peer-probe connect --device H-B --address 192.0.2.10:24876 --server-name 192.0.2.10 --ca .\secrets\connector\ca.pem --cert .\secrets\connector\identity.pem --key .\secrets\connector\identity-key.pem --output .\reports\H-B-peer.json
```

두 report 모두 `completed=true`, `authenticated=true`, `sessionsCompleted=2`, `freshIdentities=true`여야 한다. report에는 주소나 session identifier가 저장되지 않는다.

## 실제 입력 agent session

먼저 `peer-probe`가 양쪽에서 통과한 같은 장비 pair에서만 진행한다. 두 노트북 모두 손이 닿는 같은 책상에 두고, 중요한 작업을 닫고, 긴급 해제 `Ctrl+Alt+Shift+F12`를 먼저 외운다. 한쪽만 원격 장소에 둔 상태에서는 실행하지 않는다.

listener 예시:

```powershell
$HeartbeatInterval = Read-Host 'heartbeat interval'
$PeerTimeout = Read-Host 'peer timeout (must be longer)'
.\krasis-lab.exe agent-session listen --device H-A --address 0.0.0.0:24876 --ca .\secrets\listener\ca.pem --cert .\secrets\listener\identity.pem --key .\secrets\listener\identity-key.pem --peer-cert .\secrets\listener\peer.pem --edge RIGHT --duration 2m --heartbeat-interval $HeartbeatInterval --peer-timeout $PeerTimeout --output .\reports\H-A-agent.json --guard-output .\reports\H-A-guard.json --allow-live-input
```

connector 예시:

```powershell
$HeartbeatInterval = Read-Host 'same heartbeat interval used by the listener'
$PeerTimeout = Read-Host 'same peer timeout used by the listener'
.\krasis-lab.exe agent-session connect --device H-B --address 192.0.2.10:24876 --server-name 192.0.2.10 --ca .\secrets\connector\ca.pem --cert .\secrets\connector\identity.pem --key .\secrets\connector\identity-key.pem --peer-cert .\secrets\connector\peer.pem --edge LEFT --duration 2m --heartbeat-interval $HeartbeatInterval --peer-timeout $PeerTimeout --output .\reports\H-B-agent.json --guard-output .\reports\H-B-guard.json --allow-live-input
```

두 값은 제품 기본값이 아니라 이번 evidence set의 실험 입력이다. 집과 스터디카페 실행에서
같은 값을 다시 입력하고 report의 millisecond 필드로 일치 여부를 검증한다. 측정 전에 특정
숫자를 제품 기본값으로 승인하지 않는다.

실행 중 A의 오른쪽 끝에서 B로 넘어가 pointer, wheel, 기본 button과 안전한 test key를 확인한다. B의 왼쪽 끝에서 A로 돌아온 뒤 반대 방향도 반복한다. key나 button을 누른 상태로 경계를 넘지 않는다. 이상 동작, stuck input 또는 예상하지 않은 suppression이 보이면 즉시 긴급 해제를 누른다.

각 방향에서 제어권을 받은 뒤 최소 두 heartbeat interval이 지나도록 active 상태를 유지한다.
이 시간은 제품 기본값이 아니라 해당 실행에서 직접 입력한 interval의 배수다. 그래야 양쪽
report에 송수신 각각 2회 이상과 같은 session 내부 receive gap sample이 남는다.

양쪽 report가 모두 `schemaVersion=krasis.lab.agent-session/v5`, `completed=true`,
`passed=true`, `crashGuard.started=true`, `crashGuard.cleanShutdown=true`,
양수 `heartbeatIntervalMillis`, 그보다 큰 `peerTimeoutMillis`, `supervisor.activeCount>=1`,
`liveness.heartbeatsSent>=2`, `liveness.heartbeatsReceived>=2`,
`liveness.receiveGapSamples>=1`,
`0<liveness.maxReceiveGapMillis<peerTimeoutMillis`,
`destinationInput.entries>=1`이어야 해당 pair의 bounded 양방향
실험이 통과한 것이다. 집 pair와 스터디카페 pair는 서로 다른 날짜에 반복해도 된다.

두 report를 한 directory에 모은 뒤 strict pair checker를 실행한다.

```powershell
.\krasis-lab.exe agent-session-check .\reports\H-A-agent.json .\reports\H-B-agent.json
```

`valid=true`, `pairPassed=true`와 exit `0`을 함께 확인한다. report를 손으로 고쳐 통과시키지 않는다. 실패 report와 긴급 해제 report도 원인 분석용으로 별도 보존한다.

## destination 강제 종료 복구

이 단계는 두 장비가 같은 책상에 있고 양쪽 `agent-session-check`가 먼저 통과한 경우에만
사람이 수동으로 수행한다. agent가 출력한 `agent_pid`를 기록한다. source에서 메모장처럼
안전한 창에 test key 하나를 누른 상태로 유지하고, destination의 별도 PowerShell에서 아래
명령으로 **agent PID만** 강제 종료한다. 이 명령은 mustflow agent 실행 권한이 없는
사람 전용 수동 절차다. guard PID를 종료하지 않는다.

```powershell
Stop-Process -Id <agent_pid> -Force
```

source에서 key를 놓고 destination의 입력이 즉시 정상인지 확인한다. guard report가 생기면
검사한다.

```powershell
.\krasis-lab.exe crash-recovery-check .\reports\H-A-recovery.json
```

강제 종료 실험에서는 처음 실행할 때 `--guard-output .\reports\H-A-recovery.json`처럼 기존
정상 종료 report와 다른 새 경로를 지정한다. `valid=true`, `reason=PARENT_EOF`,
`attempted>=1`, `released=attempted`, `remaining=0`이어야 한다. H-A, H-B를 각각 destination으로
한 번씩 반복하고, 다른 날 스터디카페에서 S-A, S-B도 같은 방식으로 반복한다.

## 순차 Phase 0 집계

집과 스터디카페에서 같은 clean build, duration, heartbeat interval과 peer timeout으로 모은 파일을 한 directory에 복사한 뒤
파일 이름을 `H-A-agent.json`, `H-B-agent.json`, `S-A-agent.json`, `S-B-agent.json`, 각 alias의
`*-guard.json`과 `*-recovery.json`으로 맞춘다. probe kit의 아래 사람 전용 helper를 실행한다.
수집 날짜가 달라도 되고 네 장비를 동시에 켤 필요는 없다.

```powershell
.\check-phase0.ps1 -ReportDirectory .\reports
```

`passed=true`와 exit `0`이 모두 나와야 이 범위의 Phase 0 evidence gate가 통과한다.

## 실패 처리

- exit `0`: 모든 단계가 완료된 보고서가 생성됨
- exit `1`: 부분 보고서가 생성됐을 수 있으므로 삭제하지 않고 함께 보관
- exit `2`: 별칭, duration 또는 output 경로 오류
- exit `3`: 현재 platform에서 지원되지 않음

JSON에는 원시 key, 입력 문자열, pointer 좌표, host name과 window title이 없어야 한다. `completed=false`도 실패 분석에 필요한 증거다.

## 아직 증명하지 않는 것

basic/guided probe는 한 장비의 hook, aggregate capture와 tagged `SendInput`만 검사한다.
`peer-probe`는 두 노트북 사이의 인증된 연결과 protocol frame 왕복만 검사한다.
`agent-session` report의 crash guard 정상 종료는 guard process와 IPC가 실행됐음을 증명하지만,
실제 input이 down인 순간 parent를 강제 종료한 복구를 증명하지 않는다. 그 강제 종료,
절전·잠금과 실제 `SendInput` release는 두 노트북이 같은 책상에 있을 때 별도 수동
실험으로 남는다.
