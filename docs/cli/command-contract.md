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
