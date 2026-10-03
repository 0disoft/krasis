# B Codex 작업 지시: g138 보호 입력 갱신과 PIN 재검증

이 문서 전체를 B 노트북 Codex에 전달한다. Krasis 소스 저장소는 필요 없다.
아래 값은 A에서 g138 빌드 완료 후 네 EXE를 각각 SHA-256으로 계산해 확인한 값이다.
g138 전달·B 설치·실제 PIN 제어는 이 문서 작성 시 아직 수행하지 않았다.

## 목표와 배포본

B의 Chrome 자동완성 Windows PIN 창에서 A의 마우스·키보드가 동작하도록,
g138의 보호 입력 설치를 한 번 갱신한 뒤 실제 입력을 확인해 주세요.
제품 코드를 수정하거나 다른 버전으로 대체하지 마세요.

- generation: `138`
- revision: `c3566d86d2066e8645fe567354966ce4bb356aa7`
- 배포 성격: 서명 없는 개발용 묶음. SHA-256 일치를 코드 서명으로 취급하지 마세요.

| 파일 | 독립 SHA-256 |
| --- | --- |
| `krasis.exe` | `09eed6e4fa317bb5d5407091dc43588f9c979249a171f2c9af969b3876c8a70c` |
| `krasis-agent.exe` | `87dbf6a9ad9728d82a6c3680c0a70fb3b4c8a39e13d60b23bb02f15534e3be72` |
| `krasis-cli.exe` | `5f2d4797e95837a04ac9891a61c7800e6e0a7fb245cb4ccf2e30bd2d5f2f29c8` |
| `krasis-install.exe` | `811e9dfbedf47bd60698364a630f10710ee797ce5e2c43c8f6d8dbc237afee12` |

## 실행 순서

1. 현재 사용자 세션의 실행 중인 `krasis.exe` 실제 경로를 읽기 전용으로 확인하세요.
   그 폴더의 `BUILD.json`, 네 EXE, `SHA256SUMS.txt`를 위 값과 대조하세요.
   g138이 아직 전달되지 않았거나 경로가 모호하면 설치를 시작하지 말고 알려 주세요.
   기본 A 전달 방식은 기기 업데이트입니다. 수동으로 옛 설치기를 찾지 마세요.
2. 같은 폴더의 CLI로 `doctor --json`, `status --json`, `version --json`을 보존하세요.
   기존 보호 설치가 다른 버전인지, 실제 agent가 PORTABLE인지 읽기 전용으로 확인하세요.
3. 같은 CLI의 `disconnect --json`이 코드 0이고 상태가 LOCAL인지 확인하세요.
   실패하거나 FAULTED이면 설치로 진행하지 말고 진단을 전송하세요.
4. 제로디에게 B 트레이의 **관리 앱 종료 · 에이전트 유지**를 한 번 누르도록 안내하세요.
   UI 프로세스 부재를 확인한 뒤 같은 CLI의 `safe-stop --json`을 실행하고,
   같은 세션의 제품 agent 프로세스도 종료됐는지 확인하세요.
   UI가 살아 있는 채로 설치하면 자동 복구가 agent를 다시 켤 수 있습니다.
   프로세스가 남으면 임의 강제 종료나 설치 반복으로 우회하지 마세요.
5. 같은 폴더의 `krasis-install.exe --install`을 **관리자 권한으로 한 번만** 실행하세요.
   제로디의 Windows 관리자 동의를 받고, stdout·stderr와 종료 코드를 B 로컬에 보존하세요.
   실행 방법은 고정된 명령 하나를 호출하는 관리자 PowerShell이어야 합니다.
   관리자 창이 뜨면 A 입력에 의존하지 말고 B 터치패드로 동의할 수 있다고 알려 주세요.
6. 실제 종료 코드 0, JSON `code:0`, `state:installed`, `diagnosticSaved:true`,
   `diagnostic.result:0`, `stage:complete`를 모두 확인하세요. 최신 support의
   installation time/result가 이번 실행과 일치해야 합니다. 과거 g118 receipt는
   이번 설치 성공의 증거가 아닙니다. JSON 부재·출력 불완전·실패면 PIN 시험을 중단하세요.
7. `KrasisInput`이 Running인지, 고정 설치 위치의 agent SHA-256이 위 g138 agent 값과
   일치하는지, input 이미지가 설치 manifest와 일치하는지 읽기 전용으로 확인하세요.
   서비스 수동 시작·재설치 반복·transaction 삭제·인증서 등록·ACL/UAC 변경은 금지합니다.
8. 같은 g138 폴더의 `krasis.exe`를 **일반 권한**으로 다시 여세요.
   CLI version에서 UI·agent 버전 일치를 확인하고 A와 연결하세요.
   `doctor --json`의 `agentLocation:PROTECTED`, `protectedInput:CONFIGURED`,
   설치 agent 검증·서비스 Running·경로 일치를 확인하세요.
   연결 뒤 `status --json`의 `inputBackend:PROTECTED`도 확인하세요.
   이것은 경로 선택이지 PIN 제어 성공 자체는 아닙니다.
9. 제로디가 A→B 이동·클릭·키보드와 B→A 복귀를 먼저 확인하도록 안내하세요.
   정상일 때만 B Chrome의 기존 PIN 창을 제로디가 직접 띄우고,
   **A 키보드 Esc로 닫기**, 다시 띄워 **A 마우스로 취소 클릭**을 각각 시험하세요.
   시험 중 Computer Use·UI 자동화는 호출하지 마세요. PIN 입력·수집·스크린샷·
   창 텍스트·키 입력 내용·클립보드 내용 수집도 하지 마세요.
10. 실제 창 반응, 연결 유지 여부, B 터치패드·내장 키보드 개입 여부를 각각 확인하세요.
    실패하면 B 장치로 창을 닫은 시점도 제로디의 보고로 구분해 기록하세요.
    같은 CLI로 새 `diagnostics export --json`, `diagnostics send A --json`을 실행하고
    `diagnostics status --json`의 SENT를 확인하세요. QUEUED를 전송 완료로 쓰지 마세요.

## 실패와 보고

실패한 설치를 자동으로 다시 실행하지 마세요. 생성된 서비스·설치 파일·transaction을
그대로 보존하고, 가능하면 검증된 g138 일반 권한 앱으로 기존 연결만 복구하세요.
연결 복구는 PIN 제어 복구와 별개입니다.

A에 보낼 보고에는 세대·revision·네 해시 일치 여부, 실제 설치 종료 코드·최종 JSON의
고정 상태/단계/Win32 오류, 서비스 상태·설치 agent 해시 일치, doctor의 보호 경로,
status의 실제 선택 경로, 두 PIN 시험 각각의 현장 결과, 수동 개입 시점,
새 진단 전송 상태, 최종 연결 상태를 넣어 주세요.
PIN·비밀번호·창 내용·SID·실제 사용자 경로·명령줄 원문은 보고하지 마세요.
B의 관리자 동의가 필요한 단계와 실제 PIN 현장 확인 외의 절차는 Codex가 수행하세요.
