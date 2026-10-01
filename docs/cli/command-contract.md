# CLI 명령 계약

## 제품 CLI `krasis-cli`

정식 제품 CLI는 주로 resident agent의 authenticated local API를 사용한다.
로컬 진단 내보내기와 업데이트 묶음 검증은 오프라인에서 동작한다.

```text
krasis-cli [--json] help
krasis-cli --help
krasis-cli [--json] relaunch
krasis-cli [--json] [--control-dir DIR] app open ALIAS
krasis-cli [--json] [--control-dir DIR] app open-status ALIAS REQUEST_ID
krasis-cli [--json] [--control-dir DIR] status
krasis-cli [--json] [--control-dir DIR] diagnostics
krasis-cli [--json] diagnostics export
krasis-cli [--json] [--control-dir DIR] diagnostics status
krasis-cli [--json] [--control-dir DIR] diagnostics send ALIAS
krasis-cli [--json] [--control-dir DIR] diagnostics collect ALIAS
krasis-cli [--json] [--control-dir DIR] diagnostics collect-status ALIAS REQUEST_ID
krasis-cli [--json] [--control-dir DIR] topology
krasis-cli [--json] [--control-dir DIR] connect ALIAS
krasis-cli [--json] [--control-dir DIR] disconnect
krasis-cli [--json] [--control-dir DIR] pair begin ALIAS --allow-inbound-control
krasis-cli [--json] [--control-dir DIR] pair confirm PAIRING_ID CODE
krasis-cli [--json] [--control-dir DIR] pair cancel PAIRING_ID
krasis-cli [--json] [--control-dir DIR] trust list
krasis-cli [--json] [--control-dir DIR] trust revoke SHA256_FINGERPRINT
krasis-cli [--json] [--control-dir DIR] safe-stop
```

`help`와 `--help`는 에이전트 없이 전체 명령을 보여준다. `help --json`은 각 명령의
에이전트 필요 여부와 상태 변경 여부를 함께 반환한다. `--control-dir`는 사용할 수 없다.

global option은 command 앞이나 뒤에 한 번만 사용할 수 있다. alias는 공백으로 감싸지 않은 유효한
UTF-8 1~64자이고 control character를 허용하지 않는다. pairing ID는 lowercase 32자리 hex, code는
6자리 숫자, fingerprint는 lowercase 64자리 SHA-256 hex다. 잘못된 값은 local API 호출 전에 거부한다.

`pair begin`의 inbound-control 승인은 생략할 수 없는 명시적 동의다. `safe-stop`은 agent와 소유한
session 전체를 안전 종료하므로 일반 연결 해제에는 `disconnect`를 사용한다. `topology`의
`UNCONFIGURED`는 배치가 저장됐다는 뜻이 아니며, CLI가 기본 edge를 추측하지 않는다.

`diagnostics export`는 실행 중인 에이전트 없이도 검증된 로컬 이벤트 기록을
`%LOCALAPPDATA%\Krasis\diagnostics\support-latest.json`에 저장한다. 기존 파일만 교체하며
네트워크 전송은 하지 않는다. `--control-dir`는 사용할 수 없다.

`diagnostics collect ALIAS`는 이 PC의 새 진단을 저장하고, 신뢰된 상대에게 상대 자신의 새
진단을 요청한다. 최대 30초 기다려 양쪽 파일이 이 PC에 저장되면 `saved`를 반환한다.
시간이 지나도 수신하지 못하면 `pending`과 요청 ID를 반환한다. 이 경우 명령을 다시 시작하지 말고
`diagnostics collect-status ALIAS REQUEST_ID`로 확인한다. 상대도 g96 이상이어야 한다.
상대가 오프라인이거나 요청이 1분 안에 처리되지 않으면 `EXPIRED`가 된다.

`relaunch`는 마지막으로 창을 연 `krasis.exe`를 다시 실행한다. 실행 중이면 기존 창을 앞으로
가져온다. 에이전트가 종료돼 있어도 사용할 수 있으며 `--control-dir`와 실행 경로 인자는 받지 않는다.
UI 시작 후 `%LOCALAPPDATA%\Krasis\launch\krasis-cli.exe`가 고정 복구 명령으로 준비된다.
같은 폴더의 `Krasis.cmd`를 더블클릭해도 앱을 열 수 있다.
`& "$env:LOCALAPPDATA\Krasis\launch\krasis-cli.exe" relaunch --json`으로 실행할 수 있다.
기록된 실행 파일이 삭제되거나 SHA-256이 달라지면 실행을 거부한다. 기록이 없는 경우에만
명령을 실행한 CLI와 같은 폴더의 `krasis.exe`를 사용한다. `launch_requested`는 프로세스 실행
요청 성공이며, 창 표시나 에이전트 연결 성공까지 보장하지 않는다.

`app open B`는 등록된 B의 관리 채널로 앱 열기를 요청한다. 상대의 에이전트가 살아 있고,
상대가 이 기기의 입력 제어를 허용한 경우에만 마지막 UI를 실행한다. 두 기기 모두 g109 이상이
필요하다. 입력 연결 해제 상태에서도 관리 채널은 사용할 수 있다. 경로나 명령어는 전송하지 않는다.
15초 뒤에도 결과가 없으면 `pending`과 요청 ID를 반환한다. 재전송 대신
`app open-status B REQUEST_ID`로 확인한다. 요청은 1분 후 만료되며 실행 결과 확인과 실제
창 표시 여부는 별개다. B의 에이전트까지 완전히 종료된 경우 B에서 고정 복구 명령을 실행해야 한다.

아래 `krasis-lab` 계약은 Phase 0 실험 표면이며 제품 CLI와 호환성 관계가 없다.

- 상태: Experimental
- 저장소 유형: cli-tool
- 실행 파일: `krasis-lab`
- 범위: Phase 0 Windows 입력 관찰

## 목적

`krasis-lab`은 제품 CLI가 아니라 Windows 입력 adapter를 선택하기 위한 폐기 가능한 실험 도구다. 명령과 JSON schema는 Phase 0 동안에만 호환성을 유지하며 정식 `krasis` CLI로 자동 승격되지 않는다.

`--output PATH.json`으로 report를 쓰는 모든 명령은 부모 directory가 미리 존재해야 하고 기존 output을
덮어쓰지 않는다. Windows에서는 volume root부터 부모 directory까지 reparse point가 없어야 하며,
검증 실패는 report를 다른 위치로 따라가 쓰지 않고 거부한다.

## 명령

### `diagnose`

현재 프로세스와 Windows 입력 환경을 읽기 전용으로 진단한다.

- 사용법: `krasis-lab diagnose [--json]`
- 입력 hook을 설치하지 않는다.
- Windows/build, architecture, session, elevation, virtual screen과 API capability를 보고한다.
- 사용자 이름, 장치 이름, 창 제목, 네트워크 주소와 원시 입력을 출력하지 않는다.

### `capture`

저수준 keyboard/mouse hook을 설치하고 event 종류별 누적 개수만 보고한다.

- 사용법: `krasis-lab capture [--json] [--duration DURATION]`
- `--duration`이 없으면 `Ctrl+C`까지 실행한다.
- `--duration`은 Go duration 형식의 양수만 허용한다.
- key 값, scan code, 입력 문자열, pointer 좌표와 대상 창을 출력하지 않는다.
- 입력을 억제하거나 변경하거나 주입하지 않는다.
- hook callback은 aggregate counter만 변경하며 출력은 별도 reporter가 수행한다.

### `selftest-inject`

`SendInput`과 low-level hook의 injected 분류가 같은 process에서 왕복하는지 확인한다.

- 사용법: `krasis-lab selftest-inject [--json]`
- selftest 전용 `dwExtraInfo` 표식을 붙인 상대 mouse move `+1/-1` 두 건만 한 번에 주입한다.
- 두 event는 click, wheel과 keyboard event를 만들지 않으며 `MOUSEEVENTF_MOVE_NOCOALESCE`를 사용한다.
- 실행 전후 cursor 위치가 다르면 강제로 되돌리지 않고 실패한다.
- low-level hook에서 injected flag와 selftest 표식이 모두 확인된 event만 관찰 성공으로 센다.
- 실행은 내부 deadline으로 제한되며 hook을 해제한 뒤 종료한다.
- `SendInput`은 UIPI 적용을 받으므로 높은 integrity level의 대상으로 주입할 수 있다는 뜻이 아니다.

이 명령은 injected 입력 식별을 검증한다. 아직 outbound 전송 경로가 없으므로 재전달 차단 전체를 증명하지 않는다.

### `stress-selftest`

같은 process에서 세션마다 새 Win32 hook thread를 만들고 `selftest-inject`를 반복해 session lifecycle과 cursor 복원을 검사한다.

- 사용법: `krasis-lab stress-selftest [--json] --duration DURATION`
- duration은 새 attempt를 시작할 수 있는 window다. 이미 시작한 마지막 attempt는 내부 selftest deadline까지 끝나므로 실제 종료는 요청 window보다 길 수 있다.
- 첫 non-interference 실패에서 즉시 중단한다.
- 실제 physical pointer 이동이 겹친 attempt는 `interferenceAttempts`로 분리하며 성공이나 제품 실패로 세지 않는다.
- clean pass가 하나도 없으면 `no_clean_attempt_completed`로 실패한다.

### `probe`

대상 노트북 한 대에서 `diagnose`, `selftest-inject`, bounded `capture`를 순서대로 실행하고 독립적인 증거 파일 하나를 만든다.

- 사용법: `krasis-lab probe --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json`
- 네 대를 동시에 실행할 필요가 없으며 각 장비 결과는 다른 날짜와 장소에서 따로 수집할 수 있다.
- `--device`는 실제 host name 대신 장비 원장의 고정 별칭만 허용한다.
- `--duration`은 aggregate input capture 구간이며 Go duration 형식의 양수만 허용한다.
- `--output`의 부모 directory는 미리 존재해야 하고 확장자는 `.json`이어야 한다.
- 기존 output은 덮어쓰지 않는다. 임시 파일을 같은 directory에 flush한 뒤 교체 없는 승격을 시도한다.
- 중간 단계가 실패해도 가능한 범위의 부분 보고서와 안정적인 `failureCode`를 기록하고 exit `1`로 종료한다.
- selftest가 실패해도 물리 입력 capture는 계속 실행해 서로 독립적인 증거를 같은 부분 보고서에 남긴다.
- 보고서에는 raw key, 입력 문자열, pointer 좌표, host name, 사용자 이름, window title과 network identifier를 넣지 않는다.

`probe`는 장비별 기초 증거 수집기다. 두 노트북 간 연결, 입력 전달, 억제, crash recovery 또는 네트워크 동작을 검증하지 않는다.

### `probe-check`

하나 이상의 `krasis.lab.probe/v1` JSON file을 strict하게 읽고 증거 유효성과 비교 가능성을 판정한다.

- 사용법: `krasis-lab probe-check [--json] REPORT.json [REPORT.json ...]`
- unknown JSON field, trailing JSON value, 1 MiB 초과 file과 schema 불일치를 거부한다.
- 출력에는 입력 file의 basename만 포함하고 전체 local path는 복제하지 않는다.
- 두 개 이상을 주면 device alias 중복, build revision, dirty build, capture duration과 완료 상태를 비교한다.
- 단일 valid report 검사는 exit `0`이다. 두 개 이상이 valid하지만 비교 불가능하면 exit `1`이다.

### `guided-probe`

입력 내용을 저장하지 않고 keyboard, pointer, wheel/buttons의 물리 event 종류를 단계별로 확인한다.

- 사용법: `krasis-lab guided-probe --device H-A|H-B|S-A|S-B --stage-duration DURATION --output PATH.json`
- 순서는 `diagnose`, `selftest-inject`, keyboard, pointer, wheel/buttons다.
- selftest가 실패해도 세 physical-input stage는 계속 실행하며 최종 보고서에 selftest 실패를 보존한다.
- 각 physical-input stage는 같은 user-specified duration을 사용한다.
- 누락된 event 종류는 stage별 stable failure code로 기록하고 나머지 stage도 계속 실행한다.
- stage 누락이 있으면 `guided_input_incomplete`와 exit `1`이지만 수집된 부분 증거는 보존한다.

### `pointer-shadow`

실제 입력을 억제하거나 전달하지 않고 Windows Raw Input mouse packet과 `WH_MOUSE_LL` 화면 좌표 delta의 aggregate 관계를 같은 bounded 구간에서 관찰한다.

- 사용법: `krasis-lab pointer-shadow --device H-A|H-B|S-A|S-B --edge LEFT|RIGHT --duration DURATION --output PATH.json`
- `--edge`는 해당 장비에서 검증할 물리 화면 경계다.
- 사용자는 cursor를 지정 edge까지 옮긴 뒤 capture가 끝날 때까지 바깥 방향으로 계속 움직인다.
- Raw Input 등록과 low-level mouse hook은 관찰에만 사용하며 입력을 억제, 주입 또는 network로 전달하지 않는다.
- report에는 raw packet, hook event와 edge signature의 aggregate count만 저장한다. pointer 좌표, delta 값의 시계열, device name, key와 window 정보는 저장하지 않는다.
- injected pointer move가 하나라도 섞이면 `injected_pointer_interference`로 실패한다.
- raw relative outward packet이 hook 기준 edge에서 계속 관찰되고, legacy hook X가 같은 edge 좌표에 머물거나 raw outward packet보다 hook outward move가 적은 edge-loss evidence가 함께 있어야 exit `0`이다.
- 이 명령은 production Raw Input source, 1000Hz buffering, suppression 또는 cross-device pointer 품질을 구현하거나 증명하지 않는다.

### `wheel-surface`

한 Windows 장비에서 `WH_MOUSE_LL`과 Raw Input이 같은 wheel 동작을 어느 입력 표면으로 노출하는지 aggregate로 구분한다. network, peer credential 또는 두 번째 노트북은 필요하지 않는다.

- 사용법: `krasis-lab wheel-surface --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json`
- `--duration`은 Go duration 형식의 양수이고, `--output` 부모 directory는 미리 존재해야 하며 확장자는 `.json`이어야 한다.
- 기존 output은 덮어쓰지 않는다. 같은 directory의 임시 file을 flush한 뒤 교체 없는 승격을 시도한다.
- capture 동안 pointer 이동, left/right click, 두 손가락 vertical/horizontal scroll을 수행한다. 물리 입력은 관찰만 하며 억제하거나 network로 전달하지 않는다.
- 시작과 종료에 각각 `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE` 상대 이동 `+1/-1` sentinel pair를 `SendInput`으로 넣는다. 총 네 event에는 Krasis 전용 tag가 있고 hook callback은 tagged `WM_MOUSEMOVE`만 관찰해 삼킨다. wheel sentinel을 쓰지 않아 Raw Input wheel evidence를 오염시키지 않는다.
- hook이 설치되지 않았거나 실행 중 제거되면 cursor가 순간 이동할 수 있다. pair 순합은 0이지만 완전히 무해하거나 보이지 않는다고 보장하지 않는다. 중요한 작업을 저장하고 테스트용 빈 화면에서 실행한다.
- report는 hook의 physical move/button과 physical/synthesized vertical/horizontal wheel count, Raw Input packet·zero-device·tagged-injected·vertical/horizontal wheel count와 signed delta 합계·절댓값 누적, sentinel requested/inserted/observed/swallowed count만 저장한다. 좌표, 개별 delta, 장치·창 이름은 저장하지 않는다.
- `passed=true`는 collection·sentinel health와 non-sentinel physical move 또는 physical button down/up 중 하나 이상이 정상이라는 뜻이다. 실제 interaction이 있으면 physical wheel count가 0인 `NEITHER`도 유효한 진단 결과로 통과할 수 있다.
- `signature.observedPath`는 `HOOK_AND_RAW`, `HOOK_ONLY`, `RAW_ONLY`, `NEITHER` 중 하나다. standalone `HOOK_AND_RAW`는 두 입력 표면의 관찰만 증명하며 production suppression·mapping·send 소유권을 정하지 않는다.

과거 `agent-session`의 wheel 0은 Raw Input wheel 부재를 증명하지 않는다. 당시 Raw mapper가 `RI_MOUSE_WHEEL`과 `RI_MOUSE_HWHEEL`을 의도적으로 무시했기 때문이다. standalone `77/77` `HOOK_AND_RAW`는 그 부재 가설을 반박하지만 aggregate count만으로 같은 OS event나 owner 분리를 증명하지 않는다.

### `touchpad-surface`

Windows 11 Precision Touchpad의 두 손가락 gesture가 touchpad-capable foreground window의
`WM_POINTER` 표면에 나타나는지 한 장비에서 aggregate로 확인한다. network, peer credential와 두 번째
노트북은 필요하지 않는다.

- 사용법: `krasis-lab touchpad-surface --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json`
- visible top-level test window가 열리고 foreground를 요구한다. 저장된 빈 테스트 환경에서 실행하고
  capture 동안 창을 전환하거나 닫지 않는다.
- Windows 11 `RegisterTouchpadCapableWindow`와 `GetPointerType`을 동적으로 확인한다. API가 없으면
  partial report와 exit `3`을 반환한다.
- capture 동안 test window 안에서 두 손가락 vertical/horizontal scroll을 각각 수행한다.
- pointer message는 aggregate count와 `PT_TOUCHPAD` 분류만 기록하고 모두 `DefWindowProc`에 넘긴다.
  legacy vertical/horizontal wheel도 별도 집계하지만 어떤 device나 변환 경로에서 왔는지는 단정하지 않는다.
- pointer ID, 좌표, delta, timestamp, device path, window handle·title과 원시 message를 report에 넣지 않는다.
- foreground loss, API/registration 실패, `PT_TOUCHPAD` pointer 미관찰, 조기 종료는 stable failure로
  보존한다. legacy wheel만 관찰된 `LEGACY_ONLY`는 유효한 부분 진단이지만 통과가 아니다.
- 이 명령은 production suppression, mapping, queue, send 또는 injection owner를 변경하거나 검증하지 않는다.

gram13 `agent-session/v12`에서 active pointer move/button은 관찰됐지만 hook·합성·Raw wheel이 모두
`0`이고 두 손가락 scroll은 local window에서 동작했다. 이 명령은 그 미관찰 표면을 좁히기 위한 다음
진단이며, 결과만으로 production owner를 바꾸지 않는다.

### `peer-probe`

같은 장소에 있는 노트북 두 대 사이에서 실제 TCP, TLS 1.3 mTLS, `krasis/1` ALPN과 ordered peer frame 계약을 검증한다.

- listener 사용법: `krasis-lab peer-probe listen --device ALIAS --address ADDRESS --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]`
- connector 사용법: `krasis-lab peer-probe connect --device ALIAS --address ADDRESS --server-name NAME --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]`
- listener와 connector는 같은 test CA가 서명한 서로 다른 identity를 사용해야 한다. connector의 `--server-name`은 listener certificate의 이름과 일치해야 한다.
- 한 번의 실행에서 새 random session ID와 epoch를 사용하는 두 개의 독립 connection을 연속으로 만든다.
- 각 connection은 lease request/grant/commit, 상대 pointer move envelope 검증, lease revoke를 수행한다. 실제 pointer를 움직이거나 입력을 주입하지 않는다.
- report는 주소, certificate 내용, fingerprint, session ID, epoch, 원시 입력을 저장하지 않는다.
- `--output`은 기존 file을 덮어쓰지 않는다. 실패 시에도 가능한 부분 report를 저장한다.
- 기본 timeout은 2분이며 양쪽에서 별도로 지정할 수 있다.

이 명령은 실제 peer network와 protocol compatibility를 검증하지만 Windows hook suppression, `SendInput`, cursor entry, crash recovery를 검증하지 않는다. 집의 두 대와 스터디카페의 두 대는 서로 다른 시점에 같은 절차로 실행해도 된다.

### `peer-credentials`

`peer-probe` 전용 단기 mTLS credential bundle을 로컬에 생성한다.

- 사용법: `krasis-lab peer-credentials --output DIRECTORY --server-name NAME`
- `DIRECTORY`는 존재하지 않아야 하며 기존 file이나 directory를 덮어쓰지 않는다.
- test CA private key는 memory에서 listener와 connector identity를 서명한 뒤 폐기하며 file로 저장하지 않는다.
- `listener/`와 `connector/`에는 각 역할의 `ca.pem`, `identity.pem`, `identity-key.pem`과 상대 역할의 공개 인증서 `peer.pem`을 생성한다. `peer.pem`에는 private key가 없다.
- Windows에서는 각 역할 directory와 `identity-key.pem`의 상속 DACL을 제거하고 현재 사용자와 `SYSTEM`에만 full control을 허용한다. private key load는 같은 DACL과 path 전체의 reparse point 부재를 다시 검증하며 불일치하면 fail-closed한다.
- identity 유효 기간은 생성 시점부터 24시간이다. 이는 제품 certificate 수명 정책이 아니다.
- identity load와 `trust import`는 현재 시각이 certificate 유효기간 밖이면 handshake나 store mutation 전에 거부한다.
- `--server-name`은 listener가 connector에서 실제로 접근할 DNS name 또는 IP 주소다.
- bundle 전체를 두 장비에 복제하지 않는다. listener에는 `listener/`만, connector에는 `connector/`만 복사한다.

이 명령은 명시적인 사용자 확인이 있는 제품 pairing을 구현하지 않는다. 생성된 key는 test 종료 뒤 삭제하고 Git, 동기화 드라이브, report 또는 support bundle에 넣지 않는다.

### `peer-credentials-protect`

다른 Windows 노트북으로 복사한 역할별 credential directory를 대상 노트북의 현재 사용자에게 다시 잠근다.

- 사용법: `krasis-lab peer-credentials-protect --directory ROLE_DIRECTORY`
- `ROLE_DIRECTORY`는 `identity-key.pem`을 직접 포함해야 한다.
- directory 또는 private key path에 reparse point가 있거나 private key가 regular file이 아니면 변경하지 않고 실패한다.
- 성공하면 directory에는 자식 상속이 있는 protected DACL을, private key에는 상속 없는 protected DACL을 적용한다. 둘 다 현재 사용자와 `SYSTEM`만 허용한다.
- 이 명령은 이미 노출됐을 수 있는 key를 다시 안전하게 만들었다고 보증하지 않는다. 신뢰할 수 없는 위치나 계정을 거친 key는 폐기하고 bundle을 다시 생성한다.

### `trust`

peer별 durable authorization record를 관리한다.

- import: `krasis-lab trust import --store DIRECTORY --peer-cert PEER.pem --alias NAME --allow-inbound-control`
- list: `krasis-lab trust list --store DIRECTORY [--json]`
- revoke: `krasis-lab trust revoke --store DIRECTORY --fingerprint SHA256`
- `DIRECTORY`는 `trust.json`과 `.trust.lock` 전용이고 현재 사용자와 `SYSTEM` 전용 protected DACL인 부모 아래 있어야 한다. Phase 0에서는 `peer-credentials-protect`를 마친 역할 directory 아래 `trust/`를 사용한다.
- import는 certificate fingerprint와 EKU 역할만 저장하며 certificate body와 private key를 복제하지 않는다. inbound capability flag를 명시하지 않은 import는 거부한다.
- fingerprint 중복, 폐기 record 재등록, 존재하지 않는 fingerprint 폐기와 repeated revoke는 거부한다.
- list는 명시적인 관리 명령이므로 fingerprint를 출력한다. report와 일반 진단 log에는 복제하지 않는다.
- mutation은 non-blocking process lock과 revision increment, flush 뒤 atomic replace를 사용한다. lock contention, 손상, unknown field, trailing JSON, schema·DACL 불일치는 fail-closed한다.
- 실행 중 `agent-session`은 각 inbound lease authorization에서 protected store를 다시 읽는다. reload 오류나 폐기 record는 기존 allowlist로 되돌아가지 않고 해당 lease를 거부한다. 이미 active인 lease를 `trust revoke`가 비동기로 종료하지는 않는다.

### `agent-session`

두 Windows 장비에서 실제 low-level hook, 경계 억제, mTLS peer session과 `SendInput` destination adapter를 bounded하게 실행하는 Phase 0 실험 명령이다.

- 사용법: `krasis-lab agent-session listen|connect --device ALIAS --address ADDRESS [--server-name NAME] --ca CA.pem --cert CERT.pem --key KEY.pem --trust-store DIRECTORY --edge LEFT|RIGHT --duration DURATION --heartbeat-interval DURATION --peer-timeout DURATION --output REPORT.json --guard-output GUARD.json [--stop-file PATH] [--rearm-distance PIXELS] --allow-live-input`
- `--allow-live-input`이 없으면 실행하지 않는다. 이 flag는 실제 keyboard/mouse event를 억제·전달·주입한다는 명시적 확인이다.
- `--duration`은 필수이며 1ms 이상의 정수 millisecond로 왕복 가능해야 한다. 무기한 실행을 허용하지 않는다.
- identity certificate의 남은 유효기간은 요청한 `--duration` 전체를 덮어야 한다. 준비 직전과 실제 run
  deadline 확정 때 이를 검사하며 부족하면 live listener·hook 실행 전에 또는 준비 cleanup을 거쳐
  `credential_lifetime_insufficient`로 실패한다. 자동 갱신하지 않으므로 새 Pair를 생성해야 한다.
- `--heartbeat-interval`과 `--peer-timeout`도 1ms 이상의 정수 millisecond이고 timeout이 interval보다 길어야 한다. 제품 기본값은 없으며 같은 Phase 0 evidence set의 네 report에 동일한 실험값을 명시한다.
- listener와 connector는 `peer-credentials`가 만든 역할별 identity만 사용한다.
- `--trust-store`는 먼저 `trust import`로 만든 전용 directory다. 시작 시 strict하게 검증하고 각 inbound lease authorization에서 다시 읽는다. TLS peer fingerprint·역할이 record와 다르거나 record가 revoked/권한 없음이면 lease를 거부하며 reload 실패도 fail-closed한다. 실행 중 폐기는 다음 lease부터 적용되지만 이미 active인 lease를 강제로 종료하지는 않는다.
- `--edge`는 이 장비에서 상대 장비로 넘어가는 화면 끝이다. 왼쪽 노트북은 일반적으로 `RIGHT`, 오른쪽 노트북은 `LEFT`다.
- `--rearm-distance`는 cursor entry inset보다 크고 현재 단일 화면 폭보다 작아야 한다. 범위를 벗어나면
  `return_boundary_config_failed`로 live hook 시작 전에 종료한다.
- `--stop-file`은 선택 사항인 로컬 안전 종료 신호다. 지정할 때는 report와 guard report가 있는 같은
  directory의 아직 존재하지 않는 세 번째 경로여야 한다. 실행 중 그 경로에 regular file이 생기면 agent는
  process kill 대신 기존 context cancellation과 `ReleaseAll` 정리 경로로 종료하고 신호 file을 소비한다.
  directory 감시 오류나 regular file이 아닌 경로는 fail-closed `stop_file_monitor_failed`로 남긴다. 이
  flag는 peer protocol이나 원격 종료 권한을 만들지 않으며 GUI launcher 전용 비밀 채널도 아니다.
- MVP대로 active monitor가 정확히 하나가 아니면 hook을 시작하지 않고 실패한다.
- source foreground GUI thread가 menu, system menu, popup menu, mouse capture 또는 move/size loop에 있거나
  key·mouse button이 눌려 있으면 boundary intent를 local에서 차단하고 request를 보내지 않는다. local
  interaction을 끝낸 뒤 cursor를 안쪽으로 rearm하고 다시 경계에 닿아야 한다. 상태 조회에 실패하면 원격
  entry를 허용하지 않고 session을 fail-closed로 종료한다.
- destination grant 전에는 source suppression을 활성화하지 않는다.
- outbound grant 뒤 source queue를 활성화한 다음 Windows 11 touchpad-capable capture window를 현재 edge
  cursor 아래 foreground로 활성화한다. active 구간의 touchpad와 일반 mouse wheel은 이 window의
  `WM_MOUSEWHEEL/HWHEEL`만 payload로 만들고 low-level hook wheel은 관찰 뒤 window로 통과시킨다.
- capture window가 foreground가 되지 않으면 `LeaseAbort`로 destination prepared entry를 rollback한다. active
  중 foreground를 잃으면 session을 실패시킨다.
  local return과 failure에서는 capture registration·window를 먼저 해제한 뒤 source queue를 끄고 이전
  foreground window를 조건부 복원한다.
- active source는 named `RegisterTouchpadCapableWindow` export가 없는 Windows build에서 시작하지 않는다.
- destination cursor entry는 `LeaseRequest`에서 prepare하지만 `LeaseGrant` 뒤에도 runtime
  `INBOUND_PENDING` 효과로 유지한다. source session과 capture가 준비된 뒤 exact `LeaseCommit`을 받아야
  `INBOUND_ACTIVE`와 cursor entry를 확정한다. `LeaseAbort`, EOF와 중간 실패에서는 현재 cursor가 확인된
  entry 좌표와 같을 때만 이전 snapshot으로 복원한다.
- physical `Ctrl+Alt+Shift+F12`는 현재 active source session을 즉시 중단한다. F12 terminator는 원격 전송하거나 로컬에서 억제하지 않으며 supervisor는 긴급 해제 뒤 재연결하지 않는다.
- 같은 executable의 별도 crash guard process가 destination down 전에 token을 ACK하고 성공한 up 뒤 제거한다. protocol v2는 새 prepare와 기존 반복 token을 구분하며, 새 press가 `NOT_APPLIED`로 확정된 경우에만 주입 없는 cancel로 기록을 제거한다. partial·unknown 결과와 실패한 release는 기록을 유지한다. guard가 먼저 종료되거나 정상 종료에서 남은 token을 해제하지 못하면 session은 실패한다.
- 숨겨진 Windows lifecycle monitor는 suspend/resume, session lock·disconnect·logoff, shutdown과
  display change를 terminal event로 처리한다. 기존 connection은 resume하지 않으며 source cleanup과
  crash guard disarm 뒤 새 실행에서만 diagnostics와 rearm을 다시 수행한다.
- 기본 rearm distance는 32px이고 activation gap은 0px다. 같은 distance를 destination entry 직후
  return detector의 히스테리시스에도 적용해, 안쪽 이동을 확인하기 전에는 entry edge의 잔여 delta로
  즉시 복귀하지 않는다. 이는 Phase 0 안전 실험값이며 제품 UX 기본값이 아니다.
- report에는 실행한 heartbeat interval과 peer timeout, 성공한 heartbeat 송수신 횟수, 같은 session 내부 receive gap sample 수와 최대 gap, 성공 input count, 입력 주입 실패 확실성 count, cursor rollback 결과 count와 supervisor state만 저장한다. 주소, certificate, session/epoch/lease, key code, pointer 좌표와 delta는 저장하지 않는다.
- `--guard-output`은 agent report와 같은 directory의 다른 새 JSON 경로여야 한다. guard는 정상 `DISARM` 또는 parent pipe `PARENT_EOF`에서 aggregate release count만 이 파일에 기록한다.

duration 또는 사용자의 취소로 안전 종료해도 인증된 active session, 성공한 heartbeat 송신·수신 각각
2회, 같은 session의 receive gap sample 하나, peer timeout보다 작은 최대 receive gap과 local input
activity evidence가 없으면 `passed=false`와 exit `1`이다. local evidence는 queued boundary intent와
성공적으로 전송한 source input의 조합 또는 destination entry와 적용된 input의 조합이다. 입력 주입 실패가 하나라도 있거나
cursor rollback 결과 합계가 시도 수와 다르거나 rollback 실패가 있으면 역시 통과하지 않는다. 성공한
복원과 사용자 cursor 이동을 보호한 미복원은 안전한 결과로 인정한다. 긴급 해제는 의도된 안전 동작이라
exit `0`이지만 `passed=true`로 바꾸지 않는다.

### `agent-session-check`

같은 장소의 두 장비에서 수집한 `krasis.lab.agent-session/v21` report가 bounded peer input 실험을 증명하는지 strict하게 판정한다. checker schema는 `krasis.lab.agent-session-check/v18`이다. v21은 source queue 체류와 destination input disposition conservation까지 검증한다.

- 사용법: `krasis-lab agent-session-check [--json] REPORT-A.json REPORT-B.json`
- 정확히 두 file만 받으며 unknown field, trailing JSON, 1MiB 초과 file과 schema 불일치를 거부한다.
- source/destination action, 주입 실패와 cursor rollback 분류 counter 합이 `uint64` 범위를 넘으면
  wrap한 값으로 pass를 계산하지 않고 해당 evidence를 거부한다.
- control-flow 단계 합계와 supervisor failure count도 overflow 없이 기존 source/destination/capture
  aggregate와 일치해야 한다. not-ready boundary drop 또는 명시적인 단계 실패가 한 건이라도 있으면
  report 내부 `passed=true`와 무관하게 거부한다. `blockedLocalMenu`와 `blockedLocalInteraction`은 request 전에 끝나는 의도된 local
  차단이라 단계 실패로 세지 않는다.
- file path는 결과에 basename만 남긴다.
- `H-A/H-B` 또는 `S-A/S-B` 조합만 같은 pair로 인정한다.
- listener/connect 역할 하나씩, 반대 edge, 같은 clean build revision, requested duration, heartbeat interval과 peer timeout을 요구한다.
- 두 report 모두 `completed=true`, `passed=true`, lifecycle monitor와 crash guard 시작·정상 종료,
  lifecycle event와 rearm 요구 없음, final supervisor `STOPPED`, authenticated active session, 최소
  liveness aggregate와 source 또는 destination input evidence를 가져야 한다. pair 전체에는 queued boundary
  intent와 성공적으로 전송한 source input, destination entry·input·return arm이 각각 최소 한 report에
  있어야 한다. 두 방향의 exact count 일치는 요구하지 않는다. 입력 주입 실패는 모두 0이어야 하고 cursor
  rollback은 모든 시도가 복원·미복원·실패 중 하나로 분류되며 실패가 없어야 한다.
- wheel pipeline은 기존 `hookObserved`, `synthesizedObserved`, `mapped`, `queueAccepted`,
  `queueRejected`, `rejectedInactive`, `rejectedInvalid`, `rejectedOverflow`, `sent`와
  `sourceInput.wheels`의 기존 단조·합계 관계를 유지한다. 여기에 active source session에서 유효한
  tag 없는 non-zero Raw wheel axis를 세는 `rawObserved`, tag 없는 physical hook move/button callback을
  세는 `activeHookMoves`와 `activeHookButtons`, capture window legacy wheel의 `windowObserved`,
  capture activation/deactivation과 focus restore 분류를 포함한다. 이 값들은 aggregate라 같은 OS event를
  correlate하거나 서로 exact equality를 요구하지 않으며 Raw Input의 suppression·payload ownership을
  뜻하지 않는다. 정상 foreground 경로가 실패한 경우에만 bounded thread-input fallback을 한 번 허용하며,
  fallback attempted는 success·attach failure·detach failure·retry API rejection·verification loss 중 정확히
  하나로 보존돼야 한다. saved-focus 복원도 정상 경로가 실패한 경우에만 대칭 fallback을 한 번 허용하며 같은
  다섯 outcome과 최종 focus 결과의 합계 관계를 보존해야 한다. source action이 있으면 capture activation이 있어야 하고 activation/deactivation과 focus
  결과 합계가 맞으며 focus restore 실패가 없어야 한다. `rawObserved>0` 또는 `windowObserved>0`인데
  `sent==0`인 report는 각각 stable unforwarded code로 실패해야 한다.
- supervisor의 session failure는 stable `component`, `phase`, `code` triple별 count로 분류한다. triple은
  중복될 수 없고 count 합은 `sessionFailures`와 같으며 `firstFailure`는 그 분포에 포함돼야 한다. raw
  error 문자열은 report나 checker output에 넣지 않는다.

report가 JSON으로 유효해도 pair 조건을 만족하지 않으면 exit `1`이다. 긴급 해제 report는 안전 동작 증거로 보존하지만 pair pass로 승격하지 않는다.

`agent-session` 자체의 runtime evidence gate도 destination을 단독 input activity로 인정할 때 entry,
return arm과 action을 모두 요구한다. destination action 또는 cursor rollback 분류 counter 합이
`uint64` 범위를 넘으면 report를 pass로 만들지 않는다.

### `crash-recovery-check`

parent agent 강제 종료 뒤 별도 guard가 남긴 report 하나를 strict하게 판정한다.

- 사용법: `krasis-lab crash-recovery-check [--json] GUARD.json`
- `PARENT_EOF`, clean build, `attempted>=1`, `released=attempted`, `remaining=0`, `passed=true`를 모두 요구한다.
- 정상 `DISARM`, 빈 recovery, protocol failure와 손으로 수정한 unknown field report는 통과하지 않는다.

### `phase0-check`

서로 다른 날짜에 모은 집 pair, 스터디카페 pair와 네 장비의 crash recovery를 하나로 판정한다.

- 사용법: `krasis-lab phase0-check [--json] --home REPORT x2 --study REPORT x2 --recovery REPORT x4`
- home은 `H-A/H-B`, study는 `S-A/S-B`여야 하며 네 agent report의 clean build revision과 duration이 같아야 한다.
- recovery는 네 alias가 정확히 한 번씩 있어야 하고 해당 agent report와 build revision이 같아야 한다.
- 수집 시각과 장소는 달라도 된다. 네 장비를 동시에 실행할 필요는 없다.
- checker의 동일 revision 비교는 executable byte identity를 증명하지 않는다. 같은 artifact 사용 여부는
  runbook의 workflow run·commit·kit checksum 기록으로 별도 확인하며, 그 기록이 없으면 checker가
  `passed=true`여도 Phase 0 전체 완료 근거가 아니다.

## 출력 방식

- 기본 출력은 사람이 읽는 누적 snapshot이다.
- `--json`은 한 줄에 하나의 JSON object를 출력한다.
- `capture`의 기본 보고 간격은 현재 실험 구현에서 1초다. 이는 제품 성능 목표가 아니라 관찰용 sampling cadence다.
- `selftest-inject`의 짧은 deadline과 sampling cadence도 제품 지연 목표가 아니라 유한한 안전 실험 경계다.
- `probe`의 duration은 장비별 수집자가 명시하며 보고서에 millisecond 단위로 기록한다. 서로 다른 duration의 결과는 동일 조건의 성능 비교로 취급하지 않는다.

## 취소와 종료

- `Ctrl+C` 또는 `--duration` 만료 시 Windows message loop에 종료 message를 보내고 hook을 해제한다.
- 종료 시 마지막 누적 snapshot을 한 번 더 보고한다.
- 입력 억제가 없으므로 crash guard와 `ReleaseAll`은 아직 이 명령의 책임이 아니다.

## 지원 경계

- 정식 지원 대상은 아직 확정되지 않았다.
- 현재 실험 구현은 Windows에서 low-level hook을 사용한다.
- 다른 운영체제에서는 명시적인 unsupported 오류로 종료한다.
- 관리자 응용프로그램, UAC와 보안 데스크톱의 동작은 아직 검증되지 않았다.

## 변경 규칙

명령, flag, JSON field 또는 exit code를 바꾸면 이 문서와 `docs/cli/output-and-exit-codes.md`, 관련 test를 함께 변경한다.
