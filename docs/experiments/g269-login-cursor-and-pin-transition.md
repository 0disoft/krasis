# g269 로그인 화면 커서 및 PIN 전환 확인

g268에서 B 로그인 전 연결이 현장에서 확인됐다. B→A 복귀 때 B 왼쪽 커서가
남았고, 잠금 화면을 A 키보드로 넘겨 PIN 화면을 열면 이후 A 입력이 막혔다.
g269은 커서 준비 좌표를 native desktop owner에서 읽도록 수정했다. PIN 전환
실패 원인은 아직 미확인이며 시작 단계·worker 종료 이유를 진단에 추가했다.

## B Codex 지시문

현재 실행 UI 폴더의 BUILD.json generation 269,
revision `9ee7c21c92dc91d21d55291457988d15c23ddcab` 및 네 EXE의 실제 SHA-256을
SHA256SUMS.txt와 아래 독립값에 대조해 주세요.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | 97ae3f2213b6de0ad5aeaffb8e889be1bbe5a9bf5d4a9a0ea6c0249ae435037e |
| krasis-agent.exe | be2db3adacd528bb5a120943a2eabf78452f87f57fcb176b1771d71375578962 |
| krasis-cli.exe | 8a9e09f8940a444df905892d1955a8fc4676c9b474fc036de5455f1efe0ca2d8 |
| krasis-install.exe | 8ec43b8f4af125cae8998295fecc4fe90f0e334086e049850c80ce1513534870 |

같은 폴더 CLI의 `doctor --json`, `status --json`으로 g269·healthy·PROTECTED·
A와 CONNECTED, 서비스 Running/Automatic을 확인합니다. 버전이 안 맞으면
재설치나 승인 재실행 없이 결과를 보고합니다. 기존 v2 승인은 유지합니다.
재부팅 전 `%ProgramFiles%\Krasis\LOGIN-RUNTIME.json`의 startedAt·time·service·
console·grant·lastWorker·lastFailure(stage, reason, Win32Error/Known)만 기록합니다.
Computer Use·UI 도구는 호출하지 않습니다.

## 제로디 현장 시험

A를 켜 둔 채 B를 한 번 직접 재부팅합니다. B의 첫 잠금 화면에서 연결을 기다립니다.
A 입력으로 B에 이동한 뒤 A 키보드로 잠금 화면을 넘겨 PIN 입력 화면을 엽니다.
이 단계 전후 A의 마우스·클릭·Esc 반응과 A 연결 상태를 따로 확인합니다.
가능하면 B→A로 돌아올 때 B 왼쪽의 커서 잔상이 사라지는지도 확인합니다.
시험 시각·성공/실패·B 장치 개입 여부만 보고하며 PIN 값·입력 내용은 기록하지 않습니다.

실패하면 B 장치로 직접 로그인해 연결을 복구합니다. 재부팅을 반복하지 않습니다.

## 로그인 후 B Codex

`doctor --json`, `status --json`과 위 고정 파일을 다시 읽습니다. 이번 부팅의
startedAt과 마지막 실패 stage/reason/숫자만 보고합니다. reason이 없으면
미확인이며 시작 실패 receipt를 PIN 전환 실패 원인으로 단정하지 않습니다.
`diagnostics send A --json`, `diagnostics status --json`에서 새 support의 SENT를
확인합니다. support의 loginRuntime이 고정 파일과 일치하는지도 확인합니다.
서비스 수동 시작·인증 재승인·보안 설정 변경·파일 삭제는 하지 않습니다.
