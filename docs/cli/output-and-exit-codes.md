# 출력과 exit code

- 상태: Experimental
- 저장소 유형: cli-tool
- 구현 기준: `internal/labcli/run.go`, `internal/lab/model.go`

## Exit code

| Code | 의미 |
| --- | --- |
| `0` | 명령 완료 또는 사용자가 요청한 정상 취소 |
| `1` | Windows API, hook, message loop 또는 출력 runtime 오류 |
| `2` | 알 수 없는 명령, flag 또는 잘못된 duration |
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

## 호환성

두 schema는 Phase 0 실험 계약이다. 정식 제품 API가 아니므로 변경될 수 있지만, 변경 시 schema version과 관련 test를 함께 검토한다.
