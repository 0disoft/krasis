# CLI 명령 계약

- 상태: Experimental
- 저장소 유형: cli-tool
- 실행 파일: `krasis-lab`
- 범위: Phase 0 Windows 입력 관찰

## 목적

`krasis-lab`은 제품 CLI가 아니라 Windows 입력 adapter를 선택하기 위한 폐기 가능한 실험 도구다. 명령과 JSON schema는 Phase 0 동안에만 호환성을 유지하며 정식 `krasis` CLI로 자동 승격되지 않는다.

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

### `peer-probe`

같은 장소에 있는 노트북 두 대 사이에서 실제 TCP, TLS 1.3 mTLS, `krasis/1` ALPN과 ordered peer frame 계약을 검증한다.

- listener 사용법: `krasis-lab peer-probe listen --device ALIAS --address ADDRESS --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]`
- connector 사용법: `krasis-lab peer-probe connect --device ALIAS --address ADDRESS --server-name NAME --ca CA.pem --cert CERT.pem --key KEY.pem --output REPORT.json [--timeout DURATION]`
- listener와 connector는 같은 test CA가 서명한 서로 다른 identity를 사용해야 한다. connector의 `--server-name`은 listener certificate의 이름과 일치해야 한다.
- 한 번의 실행에서 새 random session ID와 epoch를 사용하는 두 개의 독립 connection을 연속으로 만든다.
- 각 connection은 lease request/grant, 상대 pointer move envelope 검증, lease revoke를 수행한다. 실제 pointer를 움직이거나 입력을 주입하지 않는다.
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

### `agent-session`

두 Windows 장비에서 실제 low-level hook, 경계 억제, mTLS peer session과 `SendInput` destination adapter를 bounded하게 실행하는 Phase 0 실험 명령이다.

- 사용법: `krasis-lab agent-session listen|connect --device ALIAS --address ADDRESS [--server-name NAME] --ca CA.pem --cert CERT.pem --key KEY.pem --peer-cert PEER.pem --edge LEFT|RIGHT --duration DURATION --heartbeat-interval DURATION --peer-timeout DURATION --output REPORT.json --guard-output GUARD.json [--rearm-distance PIXELS] --allow-live-input`
- `--allow-live-input`이 없으면 실행하지 않는다. 이 flag는 실제 keyboard/mouse event를 억제·전달·주입한다는 명시적 확인이다.
- `--duration`은 필수이며 1ms 이상의 정수 millisecond로 왕복 가능해야 한다. 무기한 실행을 허용하지 않는다.
- `--heartbeat-interval`과 `--peer-timeout`도 1ms 이상의 정수 millisecond이고 timeout이 interval보다 길어야 한다. 제품 기본값은 없으며 같은 Phase 0 evidence set의 네 report에 동일한 실험값을 명시한다.
- listener와 connector는 `peer-credentials`가 만든 역할별 identity만 사용한다.
- `--peer-cert`는 같은 역할 directory의 `peer.pem`이어야 한다. TLS가 제시한 peer fingerprint와 역할이 이 공개 인증서에서 만든 immutable in-memory trust grant와 일치하지 않거나 grant가 revoked/권한 없음이면 lease를 거부한다.
- `--edge`는 이 장비에서 상대 장비로 넘어가는 화면 끝이다. 왼쪽 노트북은 일반적으로 `RIGHT`, 오른쪽 노트북은 `LEFT`다.
- MVP대로 active monitor가 정확히 하나가 아니면 hook을 시작하지 않고 실패한다.
- destination grant 전에는 source suppression을 활성화하지 않는다.
- physical `Ctrl+Alt+Shift+F12`는 현재 active source session을 즉시 중단한다. F12 terminator는 원격 전송하거나 로컬에서 억제하지 않으며 supervisor는 긴급 해제 뒤 재연결하지 않는다.
- 같은 executable의 별도 crash guard process가 destination down 전에 token을 ACK하고 성공한 up 뒤 제거한다. guard가 먼저 종료되거나 정상 종료에서 남은 token을 해제하지 못하면 session은 실패한다.
- 기본 rearm distance는 32px이고 activation gap은 0px다. 이는 Phase 0 안전 실험값이며 제품 UX 기본값이 아니다.
- report에는 실행한 heartbeat interval과 peer timeout, 성공한 heartbeat 송수신 횟수, 같은 session 내부 receive gap sample 수와 최대 gap, aggregate input count와 supervisor state만 저장한다. 주소, certificate, session/epoch/lease, key code, pointer 좌표와 delta는 저장하지 않는다.
- `--guard-output`은 agent report와 같은 directory의 다른 새 JSON 경로여야 한다. guard는 정상 `DISARM` 또는 parent pipe `PARENT_EOF`에서 aggregate release count만 이 파일에 기록한다.

duration 또는 사용자의 취소로 안전 종료해도 인증된 active session, 성공한 heartbeat 송신·수신 각각 2회, 같은 session의 receive gap sample 하나, peer timeout보다 작은 최대 receive gap과 destination entry·input evidence가 모두 없으면 `passed=false`와 exit `1`이다. 긴급 해제는 의도된 안전 동작이라 exit `0`이지만 `passed=true`로 바꾸지 않는다.

### `agent-session-check`

같은 장소의 두 장비에서 수집한 `krasis.lab.agent-session/v5` report가 bounded 양방향 실험을 증명하는지 strict하게 판정한다.

- 사용법: `krasis-lab agent-session-check [--json] REPORT-A.json REPORT-B.json`
- 정확히 두 file만 받으며 unknown field, trailing JSON, 1MiB 초과 file과 schema 불일치를 거부한다.
- file path는 결과에 basename만 남긴다.
- `H-A/H-B` 또는 `S-A/S-B` 조합만 같은 pair로 인정한다.
- listener/connect 역할 하나씩, 반대 edge, 같은 clean build revision, requested duration, heartbeat interval과 peer timeout을 요구한다.
- 두 report 모두 `completed=true`, `passed=true`, crash guard 시작·정상 종료, final supervisor `STOPPED`, authenticated active session, 최소 liveness aggregate와 destination entry·input·return arm evidence를 가져야 한다.

report가 JSON으로 유효해도 pair 조건을 만족하지 않으면 exit `1`이다. 긴급 해제 report는 안전 동작 증거로 보존하지만 pair pass로 승격하지 않는다.

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
