# g270 잠금 커튼에서 PIN 화면으로 전환

g269 B의 이번 부팅은 PIN 입력 전에도 `USER_SESSION`이었다.
일반 agent에 `fault_source_pointer_snapshot_failed` 및
`lifecycle_desktop_switch`가 기록됐다. service의 `start / launch` 실패는
부팅 직후 기록으로, PIN 전환 순간의 로그인 worker 종료로 해석하지 않는다.

g270은 인증된 설치형 입력 worker가 활성 Default/Winlogon을 따라가게 한다.
secure desktop에서 일반 agent의 로컬 capture를 쉬고 원격 수신을 유지한다.
기존 DesktopSwitch 재시작 경로는 유지하므로 즉시 무중단 입력은 아직 미확인이다.

## B Codex에 전달

Computer Use·UI 도구를 호출하지 않고 아래 작업을 수행해 주세요.
제로디가 B에 직접 로그인한 뒤 실행합니다. 재부팅은 자동으로 하지 않습니다.

1. 실행 UI 폴더 BUILD.json의 generation `270`, revision
   `d66432096e647b71df07118fa349771c85bb0a13` 및 아래 독립 SHA-256을 대조합니다.
   전달 완료만으로 새 버전 실행이나 보호 입력 갱신을 성공으로 처리하지 않습니다.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | 2ffc21f7d4121749fd74ab8b5a98a41dad7dff194d9c812e7cbfd90b67e6e028 |
| krasis-agent.exe | 3fc5bd1f1470a86e3439e1098fbd8a25ca84a6270bfa3a2fca23c1be7f8563b4 |
| krasis-cli.exe | 719f9925e97a8f53d63d81206db061128a7f4e27255c95cac3824cfc46959134 |
| krasis-install.exe | c4c5ae0dc41bedb8a016f360f26c51ea0c3d514ef7aeb378aba7cc969fdd21b5 |

2. 같은 폴더 CLI의 `doctor --json`, `status --json`을 확인합니다.
   설치된 agent revision이 다르면 관리 앱·agent를 정상 종료한 뒤 같은 폴더의
   `krasis-install.exe --install`을 관리자 확인으로 한 번만 실행합니다.
   안전 종료·설치 결과를 확보하지 못하면 재시도하지 않고 보고합니다.
   서비스 수동 시작·transaction 삭제·보안 설정 변경·grant 재승인은 하지 않습니다.
3. 설치 receipt의 이번 실행 시각·result `0`·stage `complete`, 고정 설치 agent의
   실제 해시 일치를 확인합니다. UI를 `krasis.exe --installed`로 독립 실행합니다.
   `doctor --json`의 버전 MATCHED·PROTECTED/CONFIGURED, `status --json`의
   A와 CONNECTED를 확인합니다. 기존 만료 없는 v2 grant는 유지합니다.
4. `%ProgramFiles%/Krasis/LOGIN-RUNTIME.json`의 startedAt·time·console·lastWorker와
   lastFailure의 stage/reason/확보된 숫자만 보존합니다.

## 제로디 시험

우선 B를 직접 잠그고 A에서 키를 눌러 PIN 화면을 엽니다. 짧은 재연결이 있으면
그 시간과 이후 이동·클릭·Esc 가능 여부를 구분합니다. PIN 화면에서 A→B 이동,
A 키보드 Esc, A→B→A 왕복을 확인합니다. PIN 값은 입력하거나 기록하지 않습니다.
성공 후에만 B를 한 번 직접 재부팅해 같은 단계 전환을 확인합니다.

입력이 안 되면 화면이 잠금 커튼으로 돌아올 때까지 기다리거나 B에서 직접
로그인해 복구합니다. 재부팅·설치 반복은 하지 않습니다.

## 시험 후

A의 CLI `diagnostics collect B --json`으로 양쪽 진단을 한 번에 받습니다.
요청이 PENDING이면 반환 ID로 `diagnostics wait B ID --timeout 45s --json`을
사용하며, 같은 요청을 다시 보내지 않습니다. B에서 수동으로 두 번 저장할 필요가
없습니다. 최신 빌드의 `installed_source_receive_only`, source pointer snapshot,
desktop switch, lease 준비/실패 기록 및 해당 시험 시각을 대조합니다.

receive-only에서는 B의 로컬 source capture를 수행하지 않습니다. B 터치패드로
A를 제어하는 경로와 secure desktop 커서 잔상 처리는 별도 수용 항목이며 이
시험의 성공으로 묶지 않습니다.
