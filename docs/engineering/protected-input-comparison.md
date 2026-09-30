# B Codex: 현재 Deskflow 입력 경로 비교

Krasis g102의 PIN 입력 문제를 조사하는 작업입니다. B(GRAM13)는 Windows
26100.9457이며 Deskflow 파일 버전은 1.26.0.0입니다. g102 시험에서는 winlogon 토큰,
SYSTEM 신원, UIAccess, 세션, Default 데스크톱, 메시지 큐, 숨은 창, 메시지 처리가 모두
확인됐지만 Esc 누름·뗌·정리 수락은 0개, 오류는 5였고 실제 PIN 창도 닫히지 않았습니다.
시험 설치와 인증서는 이미 제거됐습니다. 제품의 PIN 제어는 아직 미해결입니다.

이번 목적은 **현재 B에서 동작하는 Deskflow의 실제 실행 조건을 확인**하는 것입니다.
Krasis 제품 코드를 수정하거나 새 입력 시험을 만들지 마세요.

## 1. 읽기 전용 실행 상태 확인

다음 고정된 이름만 대상으로 프로세스와 설치된 실행 파일을 확인하세요.

- `deskflow.exe`, `deskflow-core.exe`, `deskflow-daemon.exe`
- `krasis.exe`, `krasis-agent.exe`

프로세스가 없으면 `not_running`으로 기록하세요. 설치된 파일만 보고 런타임 권한을
추정하지 마세요. 시작 인수, 환경 변수 전체, 일반 로그, 프로세스 메모리, 사용자 파일은
읽지 마세요. 프로그램이 실제 사용한 파일의 버전·SHA-256·서명 상태만 확인하세요.
서명 상태를 얻지 못하면 미확인으로 표시하고 인증서나 신뢰 저장소를 변경하지 마세요.

현재 콘솔 세션에서 프로세스 핸들을 `PROCESS_QUERY_LIMITED_INFORMATION`, 토큰을
`TOKEN_QUERY`로만 열어 아래 메타데이터를 읽으세요. 필요한 경우 관리자 PowerShell에서
로컬로 수행하되 SYSTEM 실행, 서비스 설치, 토큰 복제, 권한 활성화는 하지 마세요.
PowerShell의 로컬 .NET/Win32 호출로 읽을 수 있습니다. 별도 도구 설치가 필요하지 않습니다.

| 항목 | 읽을 API 또는 토큰 정보 |
| --- | --- |
| PID·프로세스 세션 | `ProcessIdToSessionId` |
| 토큰 세션 | `GetTokenInformation(TokenSessionId)` |
| UIAccess | `GetTokenInformation(TokenUIAccess)` |
| 실제 무결성 수준 RID | `GetTokenInformation(TokenIntegrityLevel)`의 mandatory-label SID 마지막 RID |
| 상승 여부·종류 | `TokenElevation`, `TokenElevationType` |
| 제한 토큰 여부 | `IsTokenRestricted` |
| AppContainer 여부 | `TokenIsAppContainer` |
| SYSTEM 여부·현재 사용자 여부 | 토큰 사용자 SID를 메모리에서 비교한 boolean만 |

SID·사용자 이름·로그온 ID·권한 목록은 출력하거나 저장하지 마세요. 무결성 RID는 숫자와
`Low/Medium/High/System` 분류만 보고하세요. 읽기에 실패한 필드는 `unknown`과 Win32
오류 숫자로 표시하세요. 실패를 `false`나 `0`으로 바꾸지 마세요. `TokenUIAccess=true`와
SYSTEM 신원만으로 `High/System` 무결성 또는 실제 입력 성공을 추정하지 마세요.

## 2. 같은 환경에서 Deskflow의 실제 반응 확인

먼저 일반 데스크톱에서 기존 Deskflow A↔B 연결이 준비됐는지 제로디와 확인하세요.
이미 설정된 신뢰·연결을 사용하고 새 연결·보안 설정을 만들지 마세요. 준비되지 않았다면
`comparison_not_ready`와 부족한 조건을 보고하고 이 단계는 건너뛰세요.

제로디가 준비됐다고 하면 Krasis 입력 연결과 Deskflow를 동시에 사용하지 않도록 기존
Krasis 입력 연결을 해제하세요. 앱이나 서비스·설정·인증서를 삭제하지 마세요.
Deskflow의 기존 설정을 변경하지 않고, 제로디가 B에서 같은 Chrome 자동완성 PIN 창을
띄운 뒤 **A의 마우스 이동과 Esc 한 번**으로 반응을 확인하도록 안내하세요.
PIN은 입력하거나 기록하지 마세요. PIN 창이 열려 있는 동안 화면 캡처나 UI 텍스트 조회를
하지 말고 실제 반응은 제로디의 현장 보고로 확인하세요.

그때 동작한 `deskflow-core.exe`의 위 메타데이터를 다시 읽어 초기 실행과 같은 프로세스·
토큰 조건이었는지 구분하세요. Esc로 창이 실제 닫혔는지, 마우스가 실제 이동했는지,
수동 닫힘이 있었는지 각각 기록하세요. 대상에 접근할 수 없으면 미확인으로 남기세요.
예전 성공 보고·설치된 버전·API 호출만으로 현재 성공을 대신하지 마세요.

종료 후 필요하면 B 터치패드로 남은 창을 닫고 기존 Krasis 연결을 복구하세요.
시험 때문에 바꾼 연결 상태만 복구하고, 기존 Deskflow 설정과 다른 서비스는 건드리지 마세요.

## 보고 형식

1. Windows 빌드·UBR, Deskflow 실제 입력 프로세스의 파일 버전·해시·서명 상태.
2. 각 고정 프로세스의 런타임 메타데이터 표. 프로세스·필드 부재와 접근 오류를 구분.
3. 현재 Deskflow의 A→B 마우스·Esc 실제 결과, 제로디 확인 여부, 수동 닫힘 시점.
4. 실행하지 못한 단계와 이유, 원래 연결 상태로 복구했는지.

이번에는 g102 시험 서비스를 재설치·재실행하거나 인증서, 보안 업데이트, UAC,
Secure Boot, 드라이버 서명 정책을 변경하지 마세요. 이 비교 결과로 다음 변경을 선택합니다.
