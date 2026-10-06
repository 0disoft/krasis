# 출력과 exit code

## 제품 CLI

`app recovery-status --json`은 `krasis.cli.recovery-status/v1`이다. `task`는
`ENABLED|DISABLED|UNKNOWN`, `code`는 `recorded|not_recorded|receipt_unavailable`이다.
유효한 마지막 기록은 `lastRun`의 시간·고정 코드·generation으로 표시한다.
`lastRun.historyIncomplete:true`면 이전 복구 기록이 손상되어 실패 이력이
일부 빠졌을 수 있다. 손상 원본은 로컬에 최근 4개까지 보존하고 전송하지 않는다.
g195 기록에는 조회 당시 `agentState`도 포함한다. 상태는
`LOCAL|CONNECTED|CONNECTING|RECONNECTING|DISCONNECTING|FAULTED|UNKNOWN`이며
구버전 기록에는 없을 수 있다. `app ensure`가 `FAULTED`를 관측하면
`agent_faulted`와 exit `7`을 반환하고 재시작하지 않는다. `agent_present`는
프로세스 존재이며, 응답 없이 소유권만 확인한 경우 `agentState:UNKNOWN`이다.
마지막 예약 실행 당시의 기록이므로 현재 연결 상태와 구분한다.
receipt 읽기 실패는 exit `7`, 성공은 exit `0`이다. task의 조회 실패는 `UNKNOWN`으로
유지한다. `doctor`의 `update_stage_cleanup` check는 `COMPLETED|FAILED|UNKNOWN`이며
정리 실패만으로 기존 입력 상태나 종료 코드를 바꾸지 않는다.

g190 앱 열기 결과의 `launch_accepted`는 작업 수락이고 최종 결과가 아니다.
`launch_requested`와 함께 같은 ID를 계속 조회한다. peer의 `busy`는 거부 결과이며
로컬 조회 경합인 `query_busy`만 재조회한다.

`app open`은 최대 70초 동안 같은 요청을 조회하며 `window_ready`에서만 exit `0`이다.
`app open-status`도 `pending`, `launch_accepted`, `launch_requested`에는 exit `7`을 반환한다.
대기 한도 만료는 수락 확인 뒤 `window_unconfirmed`, 수락 미확인 시 `outcome_unknown`이다.
요청 전 초기 조회의 한도 만료는 `timeout`, 대기 취소는 `canceled`이며 모두 exit `7`이다.
응답 소실·다른 요청 ID·알 수 없는 응답 코드도 `outcome_unknown`과 exit `7`이다.
받은 요청 ID는 보존한다. 시간 초과와 취소는 상대 실행 작업의 취소를 뜻하지 않으며,
CLI가 실행 요청을 다시 보내지 않는다.

`app repair --json`의 schema는 `krasis.cli.app-repair/v1`이다. 성공은
`code:recovery_ready`, `generation`과 exit `0`이다. 묶음 누락·검증 실패·CLI 빌드
불일치는 `bundle_missing`, `bundle_invalid`, `bundle_version_mismatch`와 exit `7`이다.
등록 실패는 `registration_failed`, 고정 `stage`와 exit `7`이다. 경로·원본 오류는 출력하지
않는다. 잘못된 인자·`--control-dir`는 파일 변경 전에 exit `2`로 거부한다.

g121부터 `clipboard status ALIAS`의 `transfer`는 파일 공유의 준비·대기·전송·수신·적용
상태와 제한된 실패 코드, bytes/total만 담는다. `SENT`는 상대 프로세스의 수신 확인이며,
`APPLIED`는 이 PC의 native clipboard 적용 후 ACK 확인이다. 상대 탐색기에서 실제 붙여넣은
성공을 뜻하지 않는다. 준비 실패는 `size_limit`, `item_limit`, `clipboard_changed`,
`prepare_failed`; 수신/적용/만료는 `receive_failed`, `apply_failed`, `expired`,
`apply_expired`다. 파일명·경로·내용·native 오류 원문은 renderer나 전송 상태에 포함하지 않는다.

human output은 상태와 안전한 복구 행동을 위한 짧은 text다. `--json`은 local API가 반환한 다음
versioned object 하나를 stdout에 출력한다.

`krasis-cli help --json`은 오프라인 명령 목록 `krasis.cli.help/v1`을 출력한다. 각 항목의
`requiresAgent`는 실행에 에이전트가 필요한지, `changesState`는 로컬 파일·에이전트·상대 기기
중 어느 곳이든 상태가 바뀔 수 있는지를 나타낸다. 실행 전 권한이나 연결 상태를 보장하지 않는다.

`krasis-cli version`은 CLI 자체의 generation/build를 agent가 없어도 출력한다. 접근 가능한
agent가 있으면 실행 중인 agent의 generation/build/protocol도 함께 표시한다. JSON은
`krasis.cli.version/v1`이다. `krasis-cli doctor`는 agent protocol과 연결 상태를 확인하며,
JSON은 `krasis.cli.doctor/v1`이다. 연결이 끊긴 상태면 exit `7`, protocol이 다르면 exit `6`이다.
g119부터 기존 필드를 유지하며 `code`, CLI/agent build, `ui`, `launchRecord`, `installedAgent`,
`service`, `agentLocation`, `protectedInput`, `checks`, `nextActions`를 추가한다. 실행 기록의
이미지 hash, 같은 세션의 UI/agent 이미지, 보호된 설치 파일과 서비스 상태·고정 실행 설정을
읽기 전용으로 확인한다. 경로·명령줄·토큰·native 오류 원문은 결과에 포함하지 않는다.
같은 generation이라도 build가 다르면 버전 불일치다. 조회 실패는 `UNKNOWN`이며 정상으로
바꾸지 않는다. 검증 실패·버전 불일치·중지/준비 중인 설치 서비스·확인 불가 항목은 exit `7`이다.
에이전트가 없어도 구조화된 JSON을 출력하며 unavailable은 exit `4`, 인증 실패는 exit `5`,
local API transport/protocol 오류는 exit `6`이다. 일반 portable 입력과 미설치 서비스는
지원되는 별도 상태로 표시하며, `protectedInput: CONFIGURED`는 예상 설치와 실행 경로가
일치한다는 뜻이다. 현재 입력 lease나 PIN 제어 성공을 증명하지 않는다. doctor는 앱/서비스를
시작하거나 권한·파일·연결을 변경하지 않으며 `nextActions`는 실행하지 않은 안내다.
두 명령의 JSON은 여러 사실을 합친 CLI projection이며 아래의 단일 local API 응답 투영과 구분한다.

g125부터 같은 세션의 UI 프로세스가 관측되지만 실행 기록이 없거나 불일치하면
`ui.state:RUNNING_UNVERIFIED`다. 프로세스 실행과 기록 검증을 분리하며 UI generation은
추측하지 않는다. 조회 실패·여러 이미지 경로는 계속 `UNKNOWN`이다. `nextActions`에는
같은 복구 code를 한 번만 포함한다. 이 진단은 창의 표시·응답 여부를 보장하지 않는다.

`krasis-cli diagnostics export`는 에이전트 없이 정제된 지원 파일을 저장한다. JSON
`krasis.cli.diagnostics-export/v1`은 `code: saved`일 때만 절대 `path`를 포함한다.
저장 실패는 `code: save_failed`, exit `1`이며 내부 경로나 원인을 출력하지 않는다.

`diagnostics send ALIAS`는 에이전트가 새 진단 파일을 정제해 신뢰된 상대에게 한 번만 전송
대기열에 넣는다. `queued`는 수신 완료가 아니다. 이미 진단 전송이 대기 중이면 새 요청을 보내지
않는다. 응답을 잃으면 `outcome_unknown`과 exit `7`을 반환하며 자동 재전송하지 않는다.
에이전트가 반환한 파일 경로는 출력하지 않는다. `diagnostics status`는 에이전트의 **최근 전역**
진단 전송 상태를 보여줄 뿐 특정 상대의 수신 확인은 아니다. 두 명령의 JSON은
`krasis.cli.diagnostic-transfer/v1`이다.

`diagnostics collect ALIAS`는 이 PC의 새 파일과 상대가 자신의 로그로 새로 만든 파일을
이 PC에 저장한다. JSON `krasis.cli.diagnostics-collection/v1`의 `saved`와 exit `0`은
`localPath`, `peerPath`가 모두 확인된 때만 반환한다. `pending`은 완료가 아니며 exit `7`이다.
`id`로 `diagnostics collect-status ALIAS ID`를 조회한다. 인증된 상대 기기 외에는
원격 요청을 보내지 않고, 응답 유실 시 START를 자동 재실행하지 않는다.
g198부터 수집 결과는 `support-request-<id>-local.json`과
`support-request-<id>-peer-<fingerprint>.json`에 보존한다. 다음 일반 내보내기·수집은
기존 요청의 파일을 교체하지 않는다. 같은 요청의 재전송도 최초 파일을 유지한다.
일반 내보내기·전송은 기존 `support-latest.json`·`support-peer-<fingerprint>.json`을 사용한다.
요청별 파일은 최대 32개 요청·합계 128MiB로 제한한다. 용량을 확보할 때 오래된
요청의 로컬·상대 파일을 함께 정리한다. 저장 중인 요청과 최근 2분 이내 요청은
유지하며, 이 파일들만으로 한도를 채우면 새 수집을 실패로 반환한다.
g201부터 수신 확인 응답이 유실되면 이미 저장한 같은 상대·요청 ID·전송 ID·내용의
재전송에 성공 응답을 다시 보낸다. 파일을 다시 저장하거나 새 요청으로 처리하지 않는다.
확인 기록은 에이전트 메모리에 최대 32개 상대, 2분 동안 유지하며 재시작하면 사라진다.
내용·식별자가 다르거나 상대의 권한이 취소됐으면 성공 응답을 재사용하지 않는다.

g128부터 `diagnostics wait ALIAS ID [--timeout DURATION]`은 기존 수집 요청의 양쪽 파일이
확인될 때까지 기다린다. 기본 2분, 허용 범위 1초~10분이다. JSON은 같은
`krasis.cli.diagnostics-collection/v1`이며 `RECEIVED`와 두 경로가 확인된 경우에만
`code:saved`, exit `0`이다. 실패·만료는 `not_received`, 응답의 ID 불일치는
`request_changed`, 에이전트 교체는 `agent_changed`, 시간 초과·취소는 `timeout`·`canceled`,
조회 응답 소실은 `outcome_unknown`과 exit `7`이다. 요청을 다시 시작하거나 수집·전송을
취소하지 않는다. `collect-status`도 응답 ID를 대조하며 다른 작업의 경로를 반환하지 않는다.

`krasis-cli update status`는 로컬 전송 상태를, `update status ALIAS`는 신뢰된 상대의
관측 버전·업데이트 수신 허용·현재 전송 상태를 조회한다. 상대 정보가 오래됐으면 버전을 추측하지
않고 `peerState: STALE` 또는 `UNKNOWN`으로 표시한다. 상대 상태 JSON은
`krasis.cli.update-status/v1`이다. `update send ALIAS`는 CLI 실행 파일과 함께 배포된
`BUILD.json` 묶음만 사용하며, g75 이전 상대에는 같은 배포 루트의 g75 전환 묶음을
선택한다. 별도 경로 지정은 지원하지 않는다. 성공 결과 `queued`는 **전송 시작**일 뿐 설치 완료가
아니다. 응답을 받지 못했을 때는 중복 전송을 피하기 위해 자동 재시도하지 않고 `update status`로
확인한다.

`krasis-cli update wait ALIAS ID [--timeout 2m] --json`은 `send` 결과의 `status.id`에
해당하는 송신 작업 하나의 적용 완료를 기다린다. 기본 2분, 허용 범위 1초~10분이다.
JSON은 `krasis.cli.update-wait/v1`이며 `code`, `transferState`, 대상 generation/revision,
전송 bytes/total을 포함한다. 상대가 인증된 관리 채널에서 정확한 빌드로 관측된 `UPDATED`만
`code:updated`, exit `0`이다. `DELIVERED`는 완료가 아니다. 실패, timeout, 취소, 에이전트
교체, 작업 ID/상대 교체, 응답 소실은 exit `7`이다. timeout은 업데이트 취소를 뜻하지 않으며,
확인 실패 뒤 자동으로 재전송하지 않는다. 이 명령은 읽기 전용이다.

초기 trust/runtime 조회 중에도 호출자의 취소·대기 한도 만료는 각각 `canceled`, `timeout`
JSON과 exit `7`을 반환한다. 이 상태 역시 상대의 적용 여부를 확정하거나 전송을 취소하지 않는다.

`krasis-cli update allow ALIAS`와 `update deny ALIAS`는 **해당 상대가 이 PC로 업데이트를
보낼 수 있는 권한**을 변경한다. 상대 PC의 수신 권한은 변경하지 않는다. 이미 원하는 상태면
요청을 다시 보내지 않고, 변경 후에는 동일 에이전트의 응답 상태를 확인한다. 응답을 잃었거나
에이전트가 바뀌면 중복 실행하지 말고 `update status ALIAS`로 확인한다. JSON은
`krasis.cli.update-permission/v1`이며 확인된 상태에서만 `receiveAllowed`를 반환한다.

`krasis-cli update verify`는 에이전트 없이 CLI와 같은 폴더의 7개 파일을 전송과 동일한
검증 규칙으로 검사한다. 다른 경로를 받지 않으며 파일·서명 오류의 세부 경로를 출력하지 않는다.
JSON은 `krasis.cli.update-verify/v1`이고 성공 시 세대·빌드·묶음 크기를 반환한다.

`krasis-cli clipboard status|on|off ALIAS`는 신뢰된 기기의 공유 상태를 조회하거나 변경한다.
`on`은 이후 복사하는 텍스트·이미지·파일과 폴더를 상대 기기에 공유할 수 있으므로 명시적으로
실행해야 한다. 현재 상태는 agent의 업데이트 상태를 읽어 판단하며, 설정 변경 후에도 다시 읽어
확인한다. 이미 원하는 상태면 재요청하지 않는다. 다른 기기가 공유 중이면 무심코 끄거나 바꾸지
않고 exit `7`로 거부한다. JSON `krasis.cli.clipboard/v1`에는 공유 내용이나 파일 경로가 없다.

- `krasis.agent.status/v1`
- `krasis.agent.diagnostics/v1`
- `krasis.agent.trust/v1`
- `krasis.agent.topology/v1`
- mutation은 `krasis.agent.operation-result/v1`

JSON decoder는 1 MiB를 초과한 response, unknown field, trailing value와 예상하지 않은 schema version을
거부한다. stderr에는 token, endpoint, control directory, HTTP body, username이나 원시 input을 출력하지
않는다.

| Code | 의미 |
| --- | --- |
| `0` | 성공적으로 완료됨 |
| `1` | 분류되지 않은 내부 실패 |
| `2` | command/option/config 사용 오류 |
| `3` | 현재 제품 정책이나 미구성 기능으로 거부됨 |
| `4` | resident agent가 실행 중이 아니거나 endpoint에 연결할 수 없음 |
| `5` | local credential 손상 또는 인증 실패 |
| `6` | transport, HTTP 또는 schema 호환 실패 |
| `7` | 이미 연결됨, active session 없음, unknown/revoked peer, timeout 등 안전 상태 충돌 |

operation JSON의 `REJECTED`나 `FAILED`도 파싱 가능한 결과로 stdout에 남으며 exit code로 성공과
구분한다. agent unavailable은 자동으로 agent를 시작하거나 stale endpoint를 추측하지 않는다.

## Phase 0 `krasis-lab`

- 상태: Experimental
- 저장소 유형: cli-tool
- 구현 기준: `internal/labcli/run.go`, `internal/lab/model.go`

## Exit code

| Code | 의미 |
| --- | --- |
| `0` | 명령 완료 또는 사용자가 요청한 정상 취소 |
| `1` | Windows API, hook, message loop 또는 출력 runtime 오류 |
| `2` | 알 수 없는 명령, flag, 장비 별칭, 잘못된 duration 또는 사용할 수 없는 output path |
| `3` | 현재 운영체제나 platform에서 지원하지 않음 |

## 오류 출력

오류는 `stderr`로 출력한다.

- `runtime_error:`: 실행 중 실패
- `unsupported:`: 지원하지 않는 platform 또는 capability
- 잘못된 사용법: 이유와 해당 명령의 usage

오류 message는 원시 입력, 사용자 이름, 창 제목 또는 환경 변수 값을 포함하면 안 된다.

## `diagnose --json`

- schema: `krasis.lab.diagnose/v1`
- 출력: 단일 JSON object

필드 집합:

- `os`, `architecture`
- `windows.major`, `windows.minor`, `windows.build`
- `process.sessionId`, `process.elevated`
- `display.monitorCount`, virtual screen 경계, mouse button 정보
- `capabilities.lowLevelKeyboardHook`
- `capabilities.lowLevelMouseHook`
- `capabilities.rawInput`
- `capabilities.sendInput`

## `capture --json`

- schema: `krasis.lab.capture/v1`
- 출력: 누적 snapshot JSON Lines

허용 필드:

- `elapsedMillis`
- keyboard의 physical/injected down/up/other 누적 개수
- mouse의 physical/injected move/button/wheel/other 누적 개수

금지 필드:

- virtual key와 scan code
- 입력 문자와 key 이름
- pointer 좌표
- window handle, 제목과 process 이름
- 사용자나 장치 식별자

## `selftest-inject --json`

- schema: `krasis.lab.selftest-inject/v1`
- 출력: 단일 JSON object

필드:

- `passed`: 전체 판정
- `sent`: `SendInput`이 수락한 selftest event 수
- `observed`: hook에서 표식까지 확인한 injected event 수
- `injectedMoves`: 표식 여부와 무관하게 hook이 관찰한 전체 injected mouse move 수
- `physicalMoves`: selftest 구간에 겹친 실제 physical pointer move 수
- `pointerRestored`: 실행 전후 cursor 위치 일치 여부
- `failureCode`: 실패한 경우의 안정적인 분류

실패 분류:

- `send_input_failed`
- `send_input_incomplete`
- `injected_events_not_observed`
- `physical_input_interference`
- `pointer_position_changed`

`passed=false`는 JSON을 정상 출력하더라도 exit `1`이다. UIPI 차단은 `SendInput` return이나 `GetLastError`만으로 정확히 구분되지 않을 수 있으므로 별도 성공으로 취급하지 않는다.

## `probe`

- schema: `krasis.lab.probe/v1`
- 출력: `--output`으로 지정한 단일 JSON file
- stdout: 수집 안내와 생성된 report path

필드:

- `deviceAlias`: `H-A`, `H-B`, `S-A`, `S-B` 중 하나
- `collectedAt`: UTC 수집 시각
- `buildRevision`, `buildModified`: 실행 binary의 Go VCS build 정보
- `requestedDurationMillis`: 요청한 capture duration
- `completed`, `failureCode`: 전체 완료 여부와 실패 stage
- `diagnostics`: `krasis.lab.diagnose/v1` 결과
- `capture`: 마지막 `krasis.lab.capture/v1` 누적 snapshot
- `selfTest`: `krasis.lab.selftest-inject/v1` 결과

상위 실패 분류:

- `diagnose_failed`
- `capture_failed`
- `capture_missing`
- `probe_canceled`
- `selftest_failed`

`completed=false`인 부분 보고서도 원인 분석에 쓰는 유효한 증거다. selftest 실패 뒤 성공한 capture가 함께 들어갈 수 있다. 동일 output path를 다시 사용하면 기존 증거를 보존하고 실패한다.

## `stress-selftest --json`

- schema: `krasis.lab.stress-selftest/v1`
- 출력: 단일 JSON object

`attempts`, `passedAttempts`, `interferenceAttempts`, `elapsedMillis`, `completed`와 마지막 selftest 결과를 포함한다. physical input 간섭 attempt는 pass 수에서 제외한다. 실제 defect의 첫 attempt와 failure code를 보존한다.

## `probe-check --json`

- schema: `krasis.lab.probe-check/v1`
- 출력: 단일 JSON object

전체 `valid`, 두 개 이상 입력의 `comparable`, stable issue code와 file별 요약을 출력한다. file 표시는 basename이며 원본 JSON 내용이나 전체 local path를 복사하지 않는다.

## `guided-probe`

- schema: `krasis.lab.guided-probe/v1`
- 출력: `--output`으로 지정한 단일 JSON file

공통 build와 diagnostics 정보, `stageDurationMillis`, selftest 결과와 세 stage를 포함한다. 각 stage는 name, pass 여부, stable failure code 배열과 aggregate capture snapshot만 가진다.

guided stage failure code:

- `keyboard_down_not_observed`
- `keyboard_up_not_observed`
- `pointer_move_not_observed`
- `wheel_not_observed`
- `mouse_button_down_not_observed`
- `mouse_button_up_not_observed`

상위 `guided_input_incomplete`는 하나 이상의 stage가 요구 event 종류를 관찰하지 못했다는 뜻이다.

## `pointer-shadow`

- schema: `krasis.lab.pointer-shadow/v1`
- 출력: `--output`으로 지정한 단일 JSON file

허용 정보는 장비 별칭, edge, build와 duration, 일반 diagnostics, Raw Input과 hook의 aggregate count뿐이다. 좌표, 개별 delta, 시간순 trajectory, device name, key와 window 정보는 포함하지 않는다.

stable failure code:

- `raw_relative_not_observed`
- `hook_physical_move_not_observed`
- `injected_pointer_interference`
- `edge_not_observed`
- `raw_outward_at_edge_not_observed`
- `legacy_edge_loss_not_observed`
- `pointer_shadow_canceled`
- `pointer_shadow_capture_failed`

`signature`는 `rawOutwardAtEdge`, `legacyXUnchangedAtEdge`, `legacyOutwardDeficit`과 최종 `observed`를 가진다. capture runtime이 정상 종료되면 `completed=true`다. 모든 characterization evidence가 있으면 `passed=true`와 exit `0`, evidence가 부족하면 부분 report를 보존하고 exit `1`이다.

## `wheel-surface`

- schema: `krasis.lab.wheel-surface/v1`
- 출력: `--output`으로 지정한 교체 불가 단일 JSON file

허용 정보는 장비·build·duration·diagnostics, hook의 physical move/button과 physical/synthesized vertical/horizontal wheel count, Raw Input의 전체·zero-device·tagged-injected·vertical/horizontal wheel packet count와 signed delta total·magnitude, sentinel의 `requested`/`inserted`/`observed`/`swallowed`뿐이다. 좌표, 개별 delta·trajectory, 장치·창·process identity, network·session·certificate 정보는 금지한다.

정상 bounded 실행은 시작·종료의 tagged relative pointer move `+1/-1` sentinel 두 쌍, 즉 `requested=inserted=observed=swallowed=4`를 요구한다. `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE`를 사용하고 hook은 tagged `WM_MOUSEMOVE`만 삼키므로 Raw Input wheel count를 만들지 않는다. hook이 없으면 cursor가 순간 이동할 수 있고 pair 순합이 0이어도 완전히 무해함을 보장하지 않는다.

`signature`는 hook health, physical interaction, physical/synthesized hook wheel의 개별 관찰 여부와 합산 `hookWheelObserved`, Raw wheel 관찰 여부를 보존한다. `observedPath`는 이 합산 hook 여부와 Raw 여부로 `HOOK_AND_RAW`, `HOOK_ONLY`, `RAW_ONLY`, `NEITHER`를 정한다. physical wheel 관찰은 pass gate가 아니다. sentinel health와 non-sentinel physical move 또는 physical button down/up 중 하나 이상이 있으면 `NEITHER`도 `passed=true`인 유효 진단 결과다.

stable failure code:

- `diagnose_failed`
- `wheel_surface_canceled`
- `wheel_surface_capture_failed`
- `sentinel_not_requested`
- `sentinel_insertion_incomplete`
- `sentinel_not_observed`
- `sentinel_not_swallowed`
- `physical_interaction_not_observed`

실패해도 가능한 aggregate observation을 새 output에 보존하고 exit `1`로 종료한다. 잘못된 별칭·duration·output은 report 없이 exit `2`, 비 Windows platform은 exit `3`이다.

## `touchpad-surface`

- schema: `krasis.lab.touchpad-surface/v1`
- 출력: `--output`으로 지정한 교체 불가 단일 JSON file

허용 정보는 장비·build·duration·diagnostics, API availability/registration, window 생성·foreground
관찰·activation/deactivation count, pointer message 종류별 count, pointer type query 성공/실패와
`PT_TOUCHPAD`/other aggregate count, legacy vertical/horizontal wheel count뿐이다. pointer ID, 좌표,
delta, timestamp, device path, window handle·title와 원시 message는 금지한다.

`signature.observedPath`는 `POINTER_AND_LEGACY`, `POINTER_ONLY`, `LEGACY_ONLY`, `NEITHER` 중 하나다.
legacy wheel은 device나 `DefWindowProc` 변환 원인을 증명하지 않는다. `passed=true`는 API와 window 등록,
foreground 유지, `PT_TOUCHPAD` pointer 관찰과 duration 완료를 모두 요구한다.

stable failure code:

- `diagnose_failed`
- `touchpad_api_unsupported`
- `touchpad_surface_canceled`
- `touchpad_surface_capture_failed`
- `touchpad_api_unavailable`
- `touchpad_window_not_registered`
- `touchpad_window_not_created`
- `touchpad_window_not_activated`
- `touchpad_window_foreground_lost`
- `touchpad_pointer_not_observed`
- `touchpad_pointer_type_unresolved`
- `touchpad_surface_closed_early`

실패나 unsupported여도 가능한 aggregate observation을 새 output에 보존한다. runtime/evidence 실패는
exit `1`, 잘못된 별칭·duration·output은 report 없이 exit `2`, API 미지원은 exit `3`이다.

## `peer-probe`

- schema: `krasis.lab.peer-probe/v1`
- 출력: 각 장비가 `--output`으로 지정한 단일 JSON file

허용 필드:

- `role`: `listen` 또는 `connect`
- `deviceAlias`, `collectedAt`, `buildRevision`, `buildModified`
- `completed`, `failureCode`
- `authenticated`, `tlsVersion`, `alpn`
- `sessionsCompleted`, `leaseRoundTrips`, `inputFramesChecked`, `freshIdentities`

금지 필드:

- local 또는 remote network address
- certificate, private key와 certificate fingerprint
- session ID, epoch와 lease ID
- pointer 좌표, key 또는 button 값

정상 완료는 TLS 1.3 mTLS와 `krasis/1` ALPN으로 인증된 새 connection 두 개가 각각 lease grant/commit,
input envelope, revoke round trip을 완료했다는 뜻이다. 인증서 load, listen/connect, 인증 또는 session 단계가
실패하면 부분 report를 저장하고 exit `1`로 종료한다.

## `peer-credentials`

- schema: root `manifest.json`의 `krasis.lab.peer-credentials/v3`
- 출력: 새 directory 안의 `manifest.json`, `listener/`, `connector/`; 각 역할 directory에는 `ca.pem`, `identity.pem`, `identity-key.pem`, 상대 공개 인증서 `peer.pem`이 있다.
- stdout: bundle directory, 만료 시각, 역할별 복사 경고

manifest 허용 필드:

- `createdAt`, `expiresAt`, `serverName`
- `listenerDirectory`, `connectorDirectory`
- `listenerPeerCertificate`, `connectorPeerCertificate`

private key나 certificate body는 manifest와 stdout에 출력하지 않는다. output directory가 이미 있거나 server name이 잘못됐거나 credential 생성이 실패하면 exit `1`이며, 새로 만들던 불완전 directory는 제거한다.

## `peer-credentials-protect`

- 성공: exit `0`, 보호한 역할 directory 경로 한 줄
- 잘못된 인자: exit `2`, usage를 stderr에 출력
- reparse point, non-regular private key, DACL 적용 또는 검증 실패: exit `1`

stdout과 stderr에는 private key, certificate body, SID 또는 DACL 전문을 출력하지 않는다.

## `trust`

- schema: `trust.json`의 `krasis.trust-store/v1`
- import 성공: exit `0`, alias, 역할, fingerprint와 inbound capability
- list 성공: exit `0`, text는 revision·updated time·peer record; `--json`은 strict store document
- revoke 성공: exit `0`, 폐기한 alias와 fingerprint
- 잘못된 인자: exit `2`
- store 없음·손상·schema 불일치·DACL 불일치·lock contention·중복 import·unknown fingerprint·repeated revoke: exit `1`

store record 필드는 `fingerprint`, `role`, `alias`, `allowInboundControl`, `createdAt`, 선택적
`revokedAt`이다. root는 `schemaVersion`, 단조 증가 `revision`, `updatedAt`, `peers`를 가진다.
certificate body, private key, 주소와 operational session 식별자는 저장하거나 출력하지 않는다.

## `agent-session`

- schema: `krasis.lab.agent-session/v21`
- 출력: `--output`으로 지정한 단일 JSON file

필드:

- `deviceAlias`, `role`, `edge`, `collectedAt`, build 정보, requested duration, heartbeat interval과 peer timeout
- `completed`, `passed`, `stopReason`, `failureCode`
- lifecycle monitor 시작·정상 종료, 정규화된 terminal event와 explicit rearm 필요 여부
- crash guard의 시작 여부와 남은 token을 해제한 정상 종료 여부
- 같은 directory에서 strict 검증할 guard report basename
- supervisor의 final state, active 횟수, acquire/session failure 횟수와 emergency stop 횟수
- supervisor의 최초 session failure를 나타내는 stable `firstFailure` triple과 같은 triple별
  `failureCounts`. triple은 중복될 수 없고 count 합은 `sessionFailures`와 같아야 한다.
- 성공한 heartbeat 송수신 횟수, 같은 session 내부 receive gap sample 수와 최대 gap millisecond
- source의 queued boundary intent와 성공적으로 전송한 key/button down/up, pointer move, wheel 누적 개수
- `sourcePipeline.wheel`의 기존 `hookObserved`, `synthesizedObserved`, `mapped`, `queueAccepted`,
  `queueRejected`, `rejectedInactive`, `rejectedInvalid`, `rejectedOverflow`, `sent`
- 같은 active source session에서 유효한 tag 없는 non-zero Raw vertical/horizontal wheel axis를 세는
  `rawObserved`, tag 없는 physical hook move/button callback을 세는 `activeHookMoves`와
  `activeHookButtons`
- active touchpad-capable capture window의 legacy wheel을 세는 `windowObserved`, capture
  activation/deactivation과 `focusRestoreSkipped`, `focusRestored`, `focusRestoreFailed`
- 첫 foreground 전환 실패 뒤 bounded fallback의 `foregroundFallbackAttempted`와 정확히 하나의 outcome인
  `foregroundFallbackSucceeded`, `foregroundFallbackAttachFailed`, `foregroundFallbackDetachFailed`,
  `foregroundFallbackApiRejected`, `foregroundFallbackVerificationLost`
- 첫 saved-focus 복원 실패 뒤 bounded fallback의 `focusRestoreFallbackAttempted`와 정확히 하나의 outcome인
  `focusRestoreFallbackSucceeded`, `focusRestoreFallbackAttachFailed`, `focusRestoreFallbackDetachFailed`,
  `focusRestoreFallbackApiRejected`, `focusRestoreFallbackVerificationLost`
- capture activation 실패 합계를 원인별로 나눈 `captureActivationSetupFailed`,
  `captureActivationApiRejected`, `captureActivationVerificationLost`
- focus 복원 실패 합계를 원인별로 나눈 `focusRestoreApiRejected`,
  `focusRestoreVerificationLost`. 두 값의 합은 반드시 `focusRestoreFailed`와 같아야 한다.
- 정확한 focus 복원 실패 뒤 capture window를 숨긴 결과를 세는 `postHideFocusAttempted`와 정확히 하나의
  outcome인 `postHideFocusRecovered`, `postHideFocusUnavailable`, `postHideFocusCaptureRetained`
- `controlFlow.boundary`의 queued, admitted, not-ready drop, `blockedLocalMenu`, `blockedLocalInteraction`,
  `blockedMouseCapture`, `blockedPressedInput`, `blockedMoveSize`와 admission failure
- `controlFlow.outbound`의 request write, grant receive/apply, source session/capture activation,
  commit/abort write와 revoke receive 단계별 성공·실패·outcome-unknown aggregate
- `controlFlow.inbound`의 request receive, return arm, `entryPrepareAttempted`, 성공한 `entryPrepared`,
  grant write, commit/abort receive, entry commit/rollback 시도와 revoke write aggregate
- `controlFlow.transport`의 local connection interrupt와 그 뒤 peer read wake-up aggregate
- destination의 entry, key/button down/up, pointer move, wheel과 return arm 누적 개수
- 입력 주입 실패의 `notApplied`, `partiallyApplied`, `outcomeUnknown`, `unclassified` 누적 개수
- cursor rollback의 `attempted`, `restored`, `notRestored`, `failed` 누적 개수

stable failure code:

- `diagnose_failed`
- `multiple_monitors_unsupported`
- `credential_load_failed`
- `credential_lifetime_insufficient`
- `trust_store_load_failed`
- `trust_policy_failed`
- `tls_config_failed`
- `listen_failed`
- `dial_config_failed`
- `crash_guard_executable_failed`
- `crash_guard_start_failed`
- `crash_guard_stopped`
- `return_boundary_config_failed`
- `crash_guard_shutdown_failed`
- `connection_source_shutdown_failed`
- `lifecycle_monitor_start_failed`
- `lifecycle_monitor_stopped`
- `lifecycle_monitor_shutdown_failed`
- `lifecycle_source_ownership_failed`
- `lifecycle_guard_ownership_failed`
- `lifecycle_suspend`
- `lifecycle_resume`
- `lifecycle_session_lock`
- `lifecycle_session_disconnect`
- `lifecycle_logoff`
- `lifecycle_shutdown`
- `lifecycle_display_change`
- `agent_session_failed`
- `stop_file_monitor_failed`
- `no_authenticated_session`
- `no_input_activity_observed`
- `insufficient_liveness_evidence`
- `unsafe_liveness_gap`
- `input_injection_failure_observed`
- `invalid_cursor_rollback_evidence`
- `cursor_rollback_failed`
- `invalid_destination_input_evidence`
- `invalid_source_pipeline_evidence`
- `invalid_control_flow_evidence`
- `invalid_failure_evidence`
- `control_flow_failure_observed`
- `boundary_not_ready_drop_observed`
- `raw_wheel_observed_without_forwarding`
- `window_wheel_observed_without_forwarding`

`stopReason`은 `duration_elapsed`, `canceled`, `emergency_release`, `lifecycle_transition`,
`failure` 중 하나다. lifecycle transition은 cleanup 뒤에도 explicit rearm을 요구하므로 exit `1`이고
pair evidence가 아니다. 긴급 해제는 exit `0`이지만 증거가 통과했다는 뜻은 아니다. raw runtime
error는 주소나 입력 정보가 섞일 수 있으므로 terminal에 복제하지 않고 stable failure code와
redacted report만 남긴다. failure triple은 source/capture activation, outbound state transition,
semantic request/grant/commit/abort/revoke write, local interrupt 뒤 read, remote EOF·truncated frame·일반 I/O,
heartbeat timeout과 기존 runtime component fallback만 허용한다. raw error 문자열이나 연결·session
식별자는 기록하지 않는다.
`controlFlow`는 단계별 aggregate이며 per-generation trace가 아니다. queued boundary는 admitted,
not-ready drop, local menu block, local interaction block 또는 admission failure로 분류되고, admitted request와
grant apply·source/capture activation·commit/abort write, inbound entry prepare 시도·commit/rollback의 합계 관계가
기존 source/destination evidence와 맞아야 한다. stage failure나
not-ready drop은 valid counter여도 clean pair evidence가 아니므로 별도 stable failure code를 남긴다.
`blockedLocalMenu`와 `blockedLocalInteraction`은 request 전 안전 차단이라 자체로 report failure가 아니다.
`blockedLocalInteraction`은 mouse capture, pressed input과 move/size subtype의 합과 같아야 한다. 구조적으로 유효한 stage
failure는 liveness 부족보다 대표 `failureCode` 우선순위가 높다.
wheel evidence counter는 독립 aggregate라 같은 OS event의 Raw/hook/window 상관관계나 counter 간 exact equality를
표현하지 않는다. `rawObserved`는 진단 evidence일 뿐 Raw Input의 suppression·mapping·send ownership이 아니다.
active outbound payload owner는 capture window의 legacy wheel이다. capture lifecycle과 focus 분류가 불완전하거나
focus 실패 합계·원인·post-hide 결과가 일치하지 않으면 `invalid_source_pipeline_evidence`다.
`focusRestoreApiRejected`는 `SetForegroundWindow`가 거절한 경우이고,
`focusRestoreVerificationLost`는 호출 성공 직후 저장한 foreground owner가 확인되지 않은 경합이다.
capture activation의 같은 두 field는 source capture window를 처음 foreground로 만드는 단계의 API 거부와
검증 상실이고, 나머지 position·registration 실패는 `captureActivationSetupFailed`다. foreground fallback은
정상 경로가 실패한 경우에만 정확히 한 번 시도하며 다섯 outcome의 합은 attempted와 같아야 한다. fallback
success는 capture activation success보다 클 수 없고 attach/detach/retry failure는 capture activation failure를
초과할 수 없다. attach와 detach 오류는 supervisor에서 각각
`source-capture/capture-activate/thread_attach_failed`,
`source-capture/capture-activate/thread_detach_failed`로 분류한다.
focus restore fallback도 다섯 outcome의 합이 attempted와 같아야 한다. 성공은 최종 `focusRestored`를
초과할 수 없고 네 failure outcome 합은 최종 `focusRestoreFailed`와 같아야 한다. 정확한 restore failure는 post-hide outcome으로
모두 보존돼야 하며 recovered와 unavailable은 non-terminal degraded return이다. capture-retained는
`source-capture/focus-release/capture_retained`, thread detach 실패는 기존
`source-capture/focus-restore/thread_detach_failed` terminal 오류다.
`rawObserved>0` 또는 `windowObserved>0`인데
`sent==0`이면 report는 `passed=false`와 해당 unforwarded failure code를 남긴다.

## `agent-session-check --json`

- schema: `krasis.lab.agent-session-check/v18`
- 출력: 두 file의 basename 기반 요약, 전체 `valid`, `pairPassed`와 stable issue code

pair issue code:

- `pair_requires_two_reports`
- `invalid_report`
- `duplicate_device_alias`
- `mixed_location_pair`
- `missing_role_pair`
- `non_opposite_edges`
- `mixed_build_revision`
- `modified_build`
- `mixed_duration`
- `mixed_heartbeat_interval`
- `mixed_peer_timeout`
- `missing_pair_source_evidence`
- `missing_pair_destination_evidence`

file issue에는 schema, alias, role, edge, timestamp, build, duration, liveness policy,
`invalid_liveness_evidence`, `insufficient_liveness_evidence`, `liveness_gap_reached_timeout`, pass
consistency, crash guard, `unsafe_destination_input_evidence`, `invalid_cursor_rollback_evidence`,
`unsafe_cursor_rollback_evidence`, `incomplete_source_evidence`, `incomplete_destination_evidence`,
`missing_input_activity_evidence`, `invalid_source_input_evidence`,
`invalid_destination_input_evidence`, `invalid_injection_failure_evidence`,
`invalid_source_pipeline_evidence`, `invalid_control_flow_evidence`, `invalid_failure_evidence`,
`control_flow_failure_observed`, `boundary_not_ready_drop_observed`,
`raw_wheel_observed_without_forwarding`, `window_wheel_observed_without_forwarding`이 포함된다. file 결과는
aggregate `sourceInput`, `sourcePipeline`, `controlFlow`와 `destinationInput`을
포함하지만 token code와 pointer 좌표는 포함하지 않는다. 각 file은 source 또는 destination 증거 하나로
valid할 수 있고, pair 전체에는 최소 한 file의 source 증거와 최소 한 file의 destination 증거가 있어야 한다.
두 file이 모두 strict valid하고 pair issue가 없을 때만 exit `0`이다.

## `crash-recovery-check --json`

- 입력 schema: `krasis.lab.crash-guard/v1`
- 출력 schema: `krasis.lab.crash-guard-check/v1`
- 출력: guard report basename, validity, issue, device/build, reason과 aggregate release count

stable issue에는 schema/JSON/file 오류, invalid alias/build/reason/count, non-passing guard,
unexpected reason, dirty build와 `no_recovery_attempt`가 포함된다. raw token code는 입력 report와
checker output 모두에 없다.

## `phase0-check --json`

- schema: `krasis.lab.phase0-check/v17`
- 출력: home/study pair checker 결과, 네 recovery checker 결과, 전체 `passed`와 issue

stable issue에는 pair cardinality/pass/alias, cross-location build/duration/heartbeat interval/peer timeout, recovery cardinality,
invalid/duplicate/missing device와 agent-recovery build mismatch가 포함된다.
`passed`는 report 내부의 clean build revision 일치만 판정하며 executable이나 kit의 SHA-256 동일성을
증명하지 않는다. artifact identity는 수동 runbook의 별도 checksum·workflow 기록이 소유한다.

## 호환성

현재 CLI가 출력하거나 저장하는 schema는 Phase 0 실험 계약이다. 정식 제품 API가 아니므로 변경될 수 있지만, 변경 시 schema version과 관련 test를 함께 검토한다.
