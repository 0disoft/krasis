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

정상 완료는 TLS 1.3 mTLS와 `krasis/1` ALPN으로 인증된 새 connection 두 개가 각각 lease, input envelope, revoke round trip을 완료했다는 뜻이다. 인증서 load, listen/connect, 인증 또는 session 단계가 실패하면 부분 report를 저장하고 exit `1`로 종료한다.

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

- schema: `krasis.lab.agent-session/v13`
- 출력: `--output`으로 지정한 단일 JSON file

필드:

- `deviceAlias`, `role`, `edge`, `collectedAt`, build 정보, requested duration, heartbeat interval과 peer timeout
- `completed`, `passed`, `stopReason`, `failureCode`
- lifecycle monitor 시작·정상 종료, 정규화된 terminal event와 explicit rearm 필요 여부
- crash guard의 시작 여부와 남은 token을 해제한 정상 종료 여부
- 같은 directory에서 strict 검증할 guard report basename
- supervisor의 final state, active 횟수, acquire/session failure 횟수와 emergency stop 횟수
- 마지막 session failure component의 stable 이름
- 성공한 heartbeat 송수신 횟수, 같은 session 내부 receive gap sample 수와 최대 gap millisecond
- source의 queued boundary intent와 성공적으로 전송한 key/button down/up, pointer move, wheel 누적 개수
- `sourcePipeline.wheel`의 기존 `hookObserved`, `synthesizedObserved`, `mapped`, `queueAccepted`,
  `queueRejected`, `rejectedInactive`, `rejectedInvalid`, `rejectedOverflow`, `sent`
- 같은 active source session에서 유효한 tag 없는 non-zero Raw vertical/horizontal wheel axis를 세는
  `rawObserved`, tag 없는 physical hook move/button callback을 세는 `activeHookMoves`와
  `activeHookButtons`
- active touchpad-capable capture window의 legacy wheel을 세는 `windowObserved`, capture
  activation/deactivation과 `focusRestoreSkipped`, `focusRestored`, `focusRestoreFailed`
- destination의 entry, key/button down/up, pointer move, wheel과 return arm 누적 개수
- 입력 주입 실패의 `notApplied`, `partiallyApplied`, `outcomeUnknown`, `unclassified` 누적 개수
- cursor rollback의 `attempted`, `restored`, `notRestored`, `failed` 누적 개수

stable failure code:

- `diagnose_failed`
- `multiple_monitors_unsupported`
- `credential_load_failed`
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
- `no_authenticated_session`
- `no_input_activity_observed`
- `insufficient_liveness_evidence`
- `unsafe_liveness_gap`
- `input_injection_failure_observed`
- `invalid_cursor_rollback_evidence`
- `cursor_rollback_failed`
- `invalid_destination_input_evidence`
- `invalid_source_pipeline_evidence`
- `raw_wheel_observed_without_forwarding`
- `window_wheel_observed_without_forwarding`

`stopReason`은 `duration_elapsed`, `canceled`, `emergency_release`, `lifecycle_transition`,
`failure` 중 하나다. lifecycle transition은 cleanup 뒤에도 explicit rearm을 요구하므로 exit `1`이고
pair evidence가 아니다. 긴급 해제는 exit `0`이지만 증거가 통과했다는 뜻은 아니다. raw runtime
error는 주소나 입력 정보가 섞일 수 있으므로 terminal에 복제하지 않고 stable failure code와
redacted report만 남긴다. `lastFailureComponent`는 `source-session`, `source-hook`,
`boundary-orchestrator`, `return-controller`, `peer-session`, `peer-liveness`, `unknown`만 허용한다.
wheel evidence counter는 독립 aggregate라 같은 OS event의 Raw/hook/window 상관관계나 counter 간 exact equality를
표현하지 않는다. `rawObserved`는 진단 evidence일 뿐 Raw Input의 suppression·mapping·send ownership이 아니다.
active outbound payload owner는 capture window의 legacy wheel이다. capture lifecycle과 focus 분류가 불완전하거나
focus 복원이 실패하면 `invalid_source_pipeline_evidence`다. `rawObserved>0` 또는 `windowObserved>0`인데
`sent==0`이면 report는 `passed=false`와 해당 unforwarded failure code를 남긴다.

## `agent-session-check --json`

- schema: `krasis.lab.agent-session-check/v10`
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
`invalid_source_pipeline_evidence`,
`raw_wheel_observed_without_forwarding`, `window_wheel_observed_without_forwarding`, `invalid_failure_component`가
포함된다. file 결과는 aggregate `sourceInput`, `sourcePipeline`과 `destinationInput`을
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

- schema: `krasis.lab.phase0-check/v10`
- 출력: home/study pair checker 결과, 네 recovery checker 결과, 전체 `passed`와 issue

stable issue에는 pair cardinality/pass/alias, cross-location build/duration/heartbeat interval/peer timeout, recovery cardinality,
invalid/duplicate/missing device와 agent-recovery build mismatch가 포함된다.
`passed`는 report 내부의 clean build revision 일치만 판정하며 executable이나 kit의 SHA-256 동일성을
증명하지 않는다. artifact identity는 수동 runbook의 별도 checksum·workflow 기록이 소유한다.

## 호환성

현재 CLI가 출력하거나 저장하는 schema는 Phase 0 실험 계약이다. 정식 제품 API가 아니므로 변경될 수 있지만, 변경 시 schema version과 관련 test를 함께 검토한다.
