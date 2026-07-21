# 장비별 probe 실행

- 상태: Experimental
- 대상: `H-A`, `H-B`, `S-A`, `S-B`
- 실행 방식: 사용자 수동 실행 전용

## 전제

GitHub Actions의 `krasis-probe-kit-windows-amd64-<commit>.zip` artifact 또는 로컬에서 검증한
`krasis-probe-kit-v21-queue-disposition-r2-windows-amd64.zip`을 한 번 준비해 두 장소에서 같은 ZIP을 사용한다.
v21 r2 로컬 package는 configured intent `krasis_probe_kit_package_v21_queue_disposition_r2`만 생성한다. 이 artifact는
서명된 release나 설치 프로그램이 아니다. Windows가 출처 경고를 표시할 수 있으며, commit과 checksum을
확인할 수 없는 복사본은 실행하지 않는다. 서로 다른 버전이나 revision의 파일을 한 pair에서 섞지 않는다.

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
`phase0-check`는 report의 clean build revision만 비교하고 이 kit checksum을 다시 계산하지 않는다.
따라서 네 장비에서 사용한 workflow run과 kit SHA-256 기록을 잃으면 checker pass만으로 동일
artifact 사용을 주장하지 않는다.

## 버튼 UI로 실행

검증된 kit에는 수동 CLI와 함께 `krasis-launcher.exe`가 들어 있다. 압축을 푼 같은 directory에서 launcher를
실행하면 긴 `agent-session` 명령 대신 버튼을 사용할 수 있다. Windows가 실행을 차단하면 checksum과 artifact
출처를 다시 확인하며 경고를 무시하도록 자동 우회하지 않는다.
launcher도 시작 시 배포 file 전체의 SHA-256을 확인하고 불일치하면 Pair와 연결을 차단한다. 따라서 버튼
경로에서는 별도 PowerShell checksum 명령이 필요 없다. 코드 서명과 GitHub artifact origin 확인을 대체하지는
않는다.

1. HP15에서 역할을 `HP15 · Listener`, 별칭을 `S-A`, edge에 대응하는 기본 역할을 유지하고 현재 HP15 LAN
   IP, port와 실행 시간을 확인한다.
2. `새 Pair 생성`을 누르면 새 listener credential이 자동으로 설정되고 복사할 `connector` directory가
   열린다. `secrets` parent가 없어도 launcher가 보호된 directory로 먼저 만든다. 열린 `connector`
   directory만 gram13의 kit 아래 `secrets\connector`로 복사한다. pair 전체나 listener private key를
   복사하지 않는다.
3. gram13에서 역할을 `gram13 · Connector`, 별칭을 `S-B`, 같은 HP15 IP와 port로 설정하고
   `Connector 준비`를 누른다.
4. HP15의 `연결 시작`을 먼저 누른 뒤 gram13에서 누른다. `안전 중지` 또는 launcher 창 닫기는 process kill이
   아니라 agent cleanup을 요청한다.
5. 실행이 끝나면 각 노트북에서 `보고서 복사`를 한 번 눌러 이번 run의 agent+guard JSON만 전달한다.
   launcher를 재시작하면 이전 run을 자동 선택하지 않으므로 `폴더 열기`로 직접 확인한다.

이 UI는 방화벽, LAN 연결, credential 복사와 두 노트북에서의 시작 순서를 자동화하지 않는다. Pair credential은
단기 실험용이며 만료됐거나 요청한 실행 종료까지 남은 수명이 부족하거나 다른 계정을 거쳤으면 새 Pair를
만든다. launcher에 문제가 있으면 아래 수동 CLI 절차가 source of truth다.

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

## pointer edge shadow

Guided와 Basic probe를 통과한 장비에서 현재 legacy hook 좌표 경로가 화면 edge의 바깥 방향 movement를 잃는지 별도로 확인한다. 이 단계도 사용자 수동 실행 전용이다. Deskflow와 다른 입력 공유·자동화 프로그램을 완전히 종료하고, 한 장비씩 실행한다.

왼쪽 장비의 오른쪽 edge 예시:

```powershell
.\krasis-lab.exe pointer-shadow --device S-A --edge RIGHT --duration 10s --output .\reports\S-A-pointer-shadow.json
```

오른쪽 장비에서는 alias를 `S-B`, edge를 `LEFT`로 바꾼다. capture 안내가 나오면 cursor를 지정 edge로 이동한 뒤 해당 edge 바깥 방향으로 mouse 또는 touchpad를 계속 움직인다. 이 명령은 로컬 입력을 억제하거나 다른 장비로 보내지 않는다.

report의 `completed=true`, `passed=true`, 빈 `failureCodes`, 양수 `raw.outwardWhileHookAtEdge`와 `signature.observed=true`를 함께 확인한다. `injected_pointer_interference`가 있으면 자동화 프로그램이 완전히 종료되지 않은 것이므로 해당 report를 보존하고 새 output으로 다시 실행한다.

이 report는 aggregate count만 가지며 pointer 좌표나 trajectory를 저장하지 않는다. `passed=true`는 legacy edge-clipping signature를 확인했다는 뜻이지 production Raw Input forwarding이나 pointer latency가 통과했다는 뜻이 아니다.

## 단일 장비 wheel surface 진단

두 손가락 scroll이 source 장비에만 남으면 두 장비 실험을 반복하지 말고 문제 장비 한 대에서 먼저 실행한다. gram13 예시는 다음과 같다.

```powershell
New-Item -ItemType Directory -Path .\reports -Force
.\krasis-lab.exe wheel-surface --device S-B --duration 10s --output .\reports\S-B-wheel-surface.json
```

pointer 이동, left/right click, 두 손가락 vertical/horizontal scroll을 수행한다. network, credential, listener와 HP15는 필요 없다. Deskflow 같은 입력 공유·자동화 프로그램은 종료한다.

시작과 종료에 각각 `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE` tagged 상대 pointer move `+1/-1` pair가 들어간다. 정상 hook은 네 `WM_MOUSEMOVE`를 모두 삼키며 wheel event를 만들지 않아 Raw Input wheel evidence를 오염시키지 않는다. hook이 없으면 cursor가 순간 이동할 수 있고 pair 순합이 0이어도 완전히 무해함을 보장할 수 없다. 중요한 작업을 저장하고 빈 테스트 화면에서 실행한다.

output은 기존 file을 덮어쓰지 않으므로 재실행에는 timestamp를 붙인 새 이름을 쓴다. 먼저 `completed=true`, `passed=true`, 빈 `failureCodes`, `signature.hookHealthConfirmed=true`, `observation.sentinel.requested=inserted=observed=swallowed=4`를 확인한다. `observedPath`는 다음처럼 해석한다.

- `HOOK_AND_RAW`: 두 표면의 관찰만 확인한다. aggregate count만으로 같은 OS event, production owner 또는 중복 방지를 결정하지 않는다.
- `HOOK_ONLY`: hook-owned 후보를 유지하되 active session callback 소실을 계속 조사한다.
- `RAW_ONLY`: active session에서 `rawObserved`와 hook health를 함께 계측한다. 이 결과만으로 Raw Input에 payload 소유권을 주지 않는다.
- `NEITHER`: 실제 scroll을 수행했다면 Precision Touchpad pointer/gesture 표면을 별도 조사한다.

`passed=true`는 wheel 관찰이 아니라 collection·sentinel health와 non-sentinel physical move 또는 button interaction의 통과다. 실제 interaction이 있으면 `NEITHER`도 유효하다. 과거 `agent-session` wheel 0도 Raw Input 부재를 증명하지 않는다. 당시 mapper가 `RI_MOUSE_WHEEL`과 `RI_MOUSE_HWHEEL`을 계측하지 않고 무시했다. standalone `77/77` `HOOK_AND_RAW`는 Raw 부재를 반박하지만 hook suppression과 Raw send 분리를 승인하지 않는다.

## Windows 11 touchpad pointer 표면 진단

active `agent-session/v12`에서 pointer move/button은 전달되지만 `hookObserved`,
`synthesizedObserved`, `rawObserved`가 모두 `0`인 동안 두 손가락 scroll이 source local window에서
동작하면 두 장비 세션을 다시 돌리지 말고 gram13 한 대에서 실행한다.

```powershell
New-Item -ItemType Directory -Path .\reports -Force
.\krasis-lab.exe touchpad-surface --device S-B --duration 15s --output .\reports\S-B-touchpad-surface.json
```

visible test window가 열리면 그 창 안에 pointer를 두고 15초 동안 vertical/horizontal 두 손가락
scroll을 각각 여러 번 수행한다. 창을 전환하거나 닫지 않는다. 이 명령은 foreground를 가져가므로
중요한 작업을 저장하고 빈 테스트 환경에서 실행한다. network, credential, listener와 HP15는 필요 없다.

다음을 함께 확인한다.

- `completed=true`, `passed=true`, 빈 `failureCodes`
- `api.registerAvailable=true`, `api.pointerTypeAvailable=true`, `api.registered=true`
- `window.created=true`, `window.foregroundObserved=true`, `window.deactivations=0`
- `pointer.touchpadConfirmed>0`
- `signature.observedPath`가 `POINTER_ONLY` 또는 `POINTER_AND_LEGACY`

`LEGACY_ONLY`는 legacy wheel의 device나 변환 원인을 증명하지 않으며 성공이 아니다. `NEITHER`면 실제
gesture를 수행했는지 확인해 새 output으로 한 번만 재실행한다. 좌표, pointer ID와 delta는 report에
없어야 한다. 이 결과는 production fix가 아니라 다음 owner 실험을 고르는 진단이다.

Raw Input move/button과 foreground touchpad-capable window-owned wheel을 결합한 production source가 포함된 새 build에서는 같은 S-A/S-B pair로 아래 `agent-session`을 다시
실행한다. shadow report 통과만으로 source 수정이 끝났다고 판정하지 않는다. pointer가 destination에서
한 방향으로 부드럽게 움직이는지, move 뒤 click 위치가 맞는지, wheel과 primary button이 한 번씩만
적용되는지, return 뒤 양쪽이 `LOCAL`인지 확인하고 양쪽 report의 queue/session/injection failure를
함께 검사한다. 1000Hz mouse 검증은 polling rate를 기록하고 별도 실행한다.

## 두 장비 peer probe

네 대를 동시에 켤 필요는 없다. 집에서는 `H-A`와 `H-B`, 스터디카페에서는 별도 날짜에 `S-A`와 `S-B`로 같은 절차를 반복한다.

먼저 test 전용 credential bundle을 만든다. `192.0.2.10`은 문서용 예시이므로 listener의 실제 LAN IP 또는 DNS name으로 바꾼다. 생성 directory는 기존에 존재하면 안 된다.

```powershell
.\krasis-lab.exe peer-credentials --output .\secrets\home-pair --server-name 192.0.2.10
```

`listener/`만 listener 노트북으로, `connector/`만 connector 노트북으로 안전하게 복사한다. 각 directory의 `peer.pem`은 상대 역할의 공개 인증서이며 allowlist fingerprint source다. bundle 전체와 상대 역할의 private key를 두 장비에 복제하지 않는다. key는 Git, report 폴더 또는 동기화 드라이브에 넣지 않고 24시간 안에 실험을 마친 뒤 삭제한다.

복사 직후 각 대상 노트북에서 자신의 역할 directory를 현재 Windows 사용자에게 다시 잠근다. 생성 PC의 사용자 SID는 대상 노트북의 SID와 다를 수 있고 일반 파일 복사는 ACL을 풀 수 있으므로 이 단계를 생략하면 `peer-probe`와 `agent-session`이 private key load를 거부한다.

```powershell
.\krasis-lab.exe peer-credentials-protect --directory .\secrets\listener
```

connector 노트북에서는 경로를 `.\secrets\connector`로 바꾼다. reparse point나 공유·동기화 directory를 쓰지 않는다. 보호 전에 신뢰할 수 없는 사용자나 위치에 노출된 key는 이 명령으로 재사용하지 말고 폐기 후 다시 생성한다.

각 대상 노트북에서 자신의 `peer.pem`을 local trust store에 한 번 import한다. alias는 상대 노트북을 구분할 표시 이름이다.

```powershell
.\krasis-lab.exe trust import --store .\secrets\listener\trust --peer-cert .\secrets\listener\peer.pem --alias H-B --allow-inbound-control
.\krasis-lab.exe trust list --store .\secrets\listener\trust
```

connector 노트북에서는 `listener`를 `connector`로, alias를 `H-A`로 바꾼다. fingerprint와 역할을
확인하지 못했거나 잘못 import했다면 store file을 손으로 편집하지 말고 해당 fingerprint를
`trust revoke`한 뒤 새 credential bundle로 다시 시작한다.

listener에서 먼저 실행한다.

```powershell
.\krasis-lab.exe peer-probe listen --device H-A --address 0.0.0.0:24876 --ca .\secrets\listener\ca.pem --cert .\secrets\listener\identity.pem --key .\secrets\listener\identity-key.pem --output .\reports\H-A-peer.json
```

신뢰하는 사설 LAN 또는 개인 hotspot에서 Windows 방화벽이 connector 주소에만 listener의 실제 IP와 port
접근을 허용하도록 확인한 뒤 connector에서 실행한다. 공용 LAN에서 주소별 방화벽 제한을 확인할 수 없으면
Phase 0 listener를 열지 않는다. 아래 주소와 server name은 문서용 예시이므로 실제 listener 주소와
certificate name으로 바꾼다.

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
.\krasis-lab.exe agent-session listen --device H-A --address 0.0.0.0:24876 --ca .\secrets\listener\ca.pem --cert .\secrets\listener\identity.pem --key .\secrets\listener\identity-key.pem --trust-store .\secrets\listener\trust --edge RIGHT --duration 2m --heartbeat-interval $HeartbeatInterval --peer-timeout $PeerTimeout --output .\reports\H-A-agent.json --guard-output .\reports\H-A-guard.json --allow-live-input
```

connector 예시:

```powershell
$HeartbeatInterval = Read-Host 'same heartbeat interval used by the listener'
$PeerTimeout = Read-Host 'same peer timeout used by the listener'
.\krasis-lab.exe agent-session connect --device H-B --address 192.0.2.10:24876 --server-name 192.0.2.10 --ca .\secrets\connector\ca.pem --cert .\secrets\connector\identity.pem --key .\secrets\connector\identity-key.pem --trust-store .\secrets\connector\trust --edge LEFT --duration 2m --heartbeat-interval $HeartbeatInterval --peer-timeout $PeerTimeout --output .\reports\H-B-agent.json --guard-output .\reports\H-B-guard.json --allow-live-input
```

두 값은 제품 기본값이 아니라 이번 evidence set의 실험 입력이다. 집과 스터디카페 실행에서
같은 값을 다시 입력하고 report의 millisecond 필드로 일치 여부를 검증한다. 측정 전에 특정
숫자를 제품 기본값으로 승인하지 않는다.

실행 중 A의 물리 mouse로 A의 오른쪽 끝에서 B로 넘어가 pointer, wheel, 기본 button과 안전한 test key를 확인한다. 이 outbound lease의 source는 B 화면에서도 계속 A이므로 같은 A mouse로 B의 왼쪽 끝에 도달해 A로 return한다. 양쪽이 `LOCAL`로 돌아온 것을 확인한 뒤에만 B의 물리 mouse 또는 touchpad로 B의 왼쪽 끝에서 별도의 B→A lease를 시작한다. destination의 물리 입력을 기존 lease의 return 조작과 섞지 않는다. key나 button을 누른 상태로 경계를 넘지 않는다. 이상 동작, stuck input 또는 예상하지 않은 suppression이 보이면 즉시 긴급 해제를 누른다.

Precision Touchpad 방향에서는 remote 화면에서 두 손가락 vertical scroll도 수행한다. source report의
`sourceInput.wheels`와 destination report의 `destinationInput.wheels`가 모두 증가해야 하며 local source
화면의 entry 경계에 남은 cursor 아래 창이 대신 scroll되면 실패다. 일반 mouse wheel도 별도로 한 번
검증해 hook과 Raw Input 중복으로 한 notch가 두 번 적용되지 않는지 확인한다.

v13에서는 outbound grant 직후 source edge cursor 아래에 작은 `24x24` capture surface가 잠깐 보일 수
있다. 이 창이 foreground를 소유해야 touchpad wheel이 전달된다. active 동안 다른 창으로 전환하지 말고,
return 뒤 surface가 사라지고 이전 source 창 focus가 복원됐는지 확인한다. foreground를 잃으면 안전하게
session failure로 끝나야 하며 같은 실행을 성공 evidence로 쓰지 않는다.

wheel 누락이나 session failure가 발생하면 source report의 `sourcePipeline.wheel`을 순서대로 본다.
`rawObserved`는 active source session에서 유효한 tag 없는 non-zero Raw vertical/horizontal wheel axis,
`activeHookMoves`와 `activeHookButtons`는 같은 구간의 tag 없는 physical hook move/button callback이다.
`windowObserved`는 active capture window가 받은 legacy vertical/horizontal wheel이다. 이 값들과 기존
hook wheel counter는 독립 aggregate라 같은 OS event를 correlate하거나 exact equality를
요구하지 않는다. Raw counter가 증가해도 Raw Input이 suppression·mapping·send를 소유한다는 뜻은 아니다.
touchpad에서 `hookObserved=0`, `rawObserved=0`, `windowObserved>0`은 예상 가능한 v13 경로다.
일반 mouse에서는 hook과 window가 같은 wheel을 각각 관찰할 수 있지만 payload는 window만 만든다.
관찰 수 합보다 작은 `mapped`는
zero·Krasis-tagged·self-test-tagged mapping 제외,
`rejectedInactive>0`은 제어권이 이미 local로 돌아온 구간, `rejectedInvalid>0`은 action 계약 오류,
`rejectedOverflow>0`은 bounded queue 포화를 뜻한다. `queueAccepted>sent`는 sender 전에 멈춘 action이다.
`rawObserved>0`인데 `sent==0`이면 report는 `raw_wheel_observed_without_forwarding`으로 실패해야 한다.
`windowObserved>0`인데 `sent==0`이면 `window_wheel_observed_without_forwarding`으로 실패해야 한다.
source action이 있으면 `captureActivations>=1`, activation과 deactivation이 같고 focus restore 결과 합이
deactivation과 같아야 한다. 실패가 있으면
`focusRestoreApiRejected+focusRestoreVerificationLost=focusRestoreFailed`여야 하며, 전자는 Windows API
거절, 후자는 API 성공 직후 foreground verification 경합을 뜻한다. v20은 실패 수만큼 post-hide focus release를
시도하고 recovered, unavailable 또는 capture-retained 중 정확히 하나를 남긴다. capture-retained 또는
focus fallback detach failure는 unsafe다.
Raw Input은 비동기 queue이므로 경계를 넘은 직후 pointer를 움직이거나 한 번 클릭해 hook health를 남기고
1초간 기다린 뒤 wheel을 조작한다. 그래도 같은 failure가 나와야 전환 직전 backlog 가능성을 배제할 수 있다.
양쪽 `supervisor.firstFailure`와 `failureCounts`를 함께 비교한다. count 합은 `sessionFailures`와 같고
`firstFailure`는 그 분포에 포함돼야 한다. source 원인으로 연결을 닫은 뒤 peer reader가 깨어난 경우는
`controlFlow.transport.localConnectionInterrupts`와 `readAfterLocalInterrupt`가 함께 증가할 수 있으므로
뒤따른 read 오류를 원인으로 뒤집지 않는다. raw 오류나 입력값을 report에 덧붙이지 않는다.

간헐적 경계 전환은 양쪽 `controlFlow`를 비교한다. source에서는
`boundary.queued=sourceInput.boundaryIntents`, `boundary.admitted=outbound.requestWriteCompleted`여야 하고
admitted, dropped-not-ready, blocked-local-menu, blocked-local-interaction과 admission-failed 합은 queued와
정확히 같아야 한다. blocked-local-interaction은 mouse-capture, pressed-input, move-size subtype 합과 같아야 한다.
`blockedLocalMenu`와 `blockedLocalInteraction`은 source foreground menu, mouse capture, pressed input 또는
move/size loop 동안 request를 보내지 않은 안전 차단이다. grant receive는 apply 성공·실패로,
apply 성공은 source session activation 성공·실패로, source session activation 성공은 capture activation
성공·실패로 분류된다. capture 성공은 `sourcePipeline.wheel.captureActivations`와 commit write 결과 합계가
같고 capture 실패는 abort write 결과 합계와 같아야 한다. destination은 모든 `entryPrepareAttempted`가 commit 또는
rollback 시도로 보존되고 `entryPrepared<=entryPrepareAttempted`,
`inbound.commitReceived=inbound.entryCommitted=destinationInput.entries`,
`inbound.returnArmSucceeded=destinationInput.returnArms`여야 한다. aggregate는 세대별 trace가 아니므로 서로
다른 세대의 counter를 억지로 한 사건으로 짝짓지 않는다.

각 방향에서 제어권을 받은 뒤 최소 두 heartbeat interval이 지나도록 active 상태를 유지한다.
이 시간은 제품 기본값이 아니라 해당 실행에서 직접 입력한 interval의 배수다. 그래야 양쪽
report에 송수신 각각 2회 이상과 같은 session 내부 receive gap sample이 남는다.

양쪽 report가 모두 `schemaVersion=krasis.lab.agent-session/v21`, `completed=true`,
`passed=true`, `crashGuard.started=true`, `crashGuard.cleanShutdown=true`,
`lifecycle.monitorStarted=true`, `lifecycle.cleanShutdown=true`, lifecycle event 없음,
`lifecycle.rearmRequired=false`,
양수 `heartbeatIntervalMillis`, 그보다 큰 `peerTimeoutMillis`, `supervisor.activeCount>=1`,
`liveness.heartbeatsSent>=2`, `liveness.heartbeatsReceived>=2`,
`liveness.receiveGapSamples>=1`,
`0<liveness.maxReceiveGapMillis<peerTimeoutMillis`이고, source report에는
`sourceInput.boundaryIntents>=1`과 하나 이상의 source input action, destination report에는
`destinationInput.entries>=1`, `destinationInput.returnArms>=1`과 하나 이상의 destination input action이
있어야 한다. 모든 `destinationInput.injectionFailures`가 `0`,
`destinationInput.cursorRollbacks.failed=0`, 그리고 rollback의 복원·미복원·실패 합계가 시도 수와
같아야 한다. `destinationPipeline.applied+retiredDrained+rejected=received`,
`destinationPipeline.applied=destinationInput` action 합이어야 하고 `rejected=0`이어야 한다.
`controlFlow` 합계가 유효하고 stage failure와 not-ready boundary drop이 모두 `0`이어야 한다.
source action이 있는 report는 capture activation/deactivation 합계, foreground fallback outcome과 focus restore 분류도
위 조건을 만족해야 한다. 또한 `sourcePipeline.queue.dequeued<=enqueued`,
`discardedStale<=dequeued`, `residenceSamples=dequeued`, source action 합이
`dequeued-discardedStale` 이하여야 해당 pair의 bounded peer input 실험이 통과한 것이다. touchpad acceptance 실행에서는
추가로 source `windowObserved`, `mapped`, `queueAccepted`, `sent`와 destination `wheels`가 모두 양수여야 한다.
한 방향만 실행해도 source와 destination
보고서가 서로 보완되면 checker 증거로 충분하다. 반대 방향 사용성은 별도 수동 검증 항목이며 exact
source/destination count 일치는 요구하지 않는다. 집 pair와 스터디카페 pair는 서로 다른 날짜에 반복해도 된다.

v21 queue/disposition acceptance에서는 먼저 menu, drag, pressed input과 move/size를 모두 닫은 상태로 최소 20회 왕복한다.
정상 foreground 획득은 `foregroundFallbackAttempted=0`일 수 있다. fallback이 필요했다면 attempted는 다섯
outcome의 합과 같고, 성공 outcome만 capture activation과 commit으로 이어져야 한다. attach, detach 또는
재시도 실패는 abort와 destination rollback으로 끝나야 하며 같은 실행에서 자동 retry loop가 생기면 실패다.
return 뒤 일반 focus 복원이 거절됐다면 `focusRestoreFallbackAttempted`가 증가하고 다섯 outcome 중 하나로
분류돼야 한다. fallback success는 최종 `focusRestored`로 끝난다. fallback까지 실패하면 post-hide outcome이
recovered 또는 unavailable이어야 하며 source hook 종료, remote EOF, 재연결과 not-ready boundary drop을 만들지
않아야 한다. capture-retained와 detach failure만 terminal이다. slow motion이 느껴지면 종료하지 말고 몇 차례 더
왕복해 `sourcePipeline.queue.maxDepth`, `maxResidenceMicros`, `discardedStale`,
`destinationPipeline.retiredDrained`와 supervisor failure distribution을 같은 report에 남긴다. 입력 좌표나 키값은
보고서에 기록되지 않는다.

## Windows lifecycle fault injection

정상 pair 실행과 다른 새 report 경로를 사용해 session을 다시 시작하고, active 상태에서 한 번에
하나씩 Windows 잠금, 절전·복귀 또는 display topology 변경을 수동으로 발생시킨다. event 뒤 기존
session이 자동으로 돌아오면 실패다. agent report는 `stopReason=lifecycle_transition`, 해당
`lifecycle_*` failure code, `lifecycle.rearmRequired=true`, supervisor `STOPPED`와 crash guard 정상
종료를 남겨야 한다. 이 report는 `agent-session-check` 성공 입력으로 쓰지 않고 fault evidence로
별도 보존한다. 잠금 해제나 절전 복귀 뒤에는 양쪽 process가 종료된 것을 확인한 다음에만 새 output
경로로 명시적으로 다시 실행한다.

두 report를 한 directory에 모은 뒤 strict pair checker를 실행한다.

```powershell
.\krasis-lab.exe agent-session-check .\reports\H-A-agent.json .\reports\H-B-agent.json
```

`valid=true`, `pairPassed=true`와 exit `0`을 함께 확인한다. report를 손으로 고쳐 통과시키지 않는다. 실패 report와 긴급 해제 report도 원인 분석용으로 별도 보존한다.

aggregate-only redacted 결과 기록 형식은
`docs/experiments/results/2026-07-16-study-pair-v13.md`를 따른다. 이 예시는 기본 touchpad wheel 전달만
통과했으며 외장 mouse, lifecycle fault와 parent 강제 종료 gate를 대신하지 않는다.

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
절전·잠금 message 수신, cleanup 뒤 명시적 rearm과 실제 `SendInput` release는 두 노트북이 같은 책상에 있을 때 별도 수동
실험으로 남는다.
