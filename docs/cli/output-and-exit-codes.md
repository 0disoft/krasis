# 출력과 exit code

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

정상 완료는 TLS 1.3 mTLS와 `krasis/1` ALPN으로 인증된 새 connection 두 개가 각각 lease, input envelope, revoke round trip을 완료했다는 뜻이다. 인증서 load, listen/connect, 인증 또는 session 단계가 실패하면 부분 report를 저장하고 exit `1`로 종료한다.

## `peer-credentials`

- schema: root `manifest.json`의 `krasis.lab.peer-credentials/v1`
- 출력: 새 directory 안의 `manifest.json`, `listener/`, `connector/`
- stdout: bundle directory, 만료 시각, 역할별 복사 경고

manifest 허용 필드:

- `createdAt`, `expiresAt`, `serverName`
- `listenerDirectory`, `connectorDirectory`

private key나 certificate body는 manifest와 stdout에 출력하지 않는다. output directory가 이미 있거나 server name이 잘못됐거나 credential 생성이 실패하면 exit `1`이며, 새로 만들던 불완전 directory는 제거한다.

## `agent-session`

- schema: `krasis.lab.agent-session/v2`
- 출력: `--output`으로 지정한 단일 JSON file

필드:

- `deviceAlias`, `role`, `edge`, `collectedAt`, build 정보와 requested duration
- `completed`, `passed`, `stopReason`, `failureCode`
- crash guard의 시작 여부와 남은 token을 해제한 정상 종료 여부
- supervisor의 final state, active 횟수, acquire/session failure 횟수와 emergency stop 횟수
- destination의 entry, key/button down/up, pointer move, wheel과 return arm 누적 개수

stable failure code:

- `diagnose_failed`
- `multiple_monitors_unsupported`
- `credential_load_failed`
- `tls_config_failed`
- `listen_failed`
- `dial_config_failed`
- `crash_guard_executable_failed`
- `crash_guard_start_failed`
- `crash_guard_stopped`
- `crash_guard_shutdown_failed`
- `agent_session_failed`
- `no_authenticated_session`
- `no_destination_input_observed`

`stopReason`은 `duration_elapsed`, `canceled`, `emergency_release`, `failure` 중 하나다. 긴급 해제는 exit `0`이지만 증거가 통과했다는 뜻은 아니다. raw runtime error는 주소나 입력 정보가 섞일 수 있으므로 terminal에 복제하지 않고 stable failure code와 redacted report만 남긴다.

## `agent-session-check --json`

- schema: `krasis.lab.agent-session-check/v1`
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

file issue에는 schema, alias, role, edge, timestamp, build, duration, pass consistency, crash guard와 destination evidence 오류가 포함된다. 두 file이 모두 strict valid하고 pair issue가 없을 때만 exit `0`이다.

## 호환성

현재 CLI가 출력하거나 저장하는 schema는 Phase 0 실험 계약이다. 정식 제품 API가 아니므로 변경될 수 있지만, 변경 시 schema version과 관련 test를 함께 검토한다.
