# g268 일반 연결 확인 후 로그인 전 시험

g267에서 B의 heartbeat 만료가 두 번 기록됐다. g268은 정상 검증·주입을 마친
현재 inbound 입력도 생존 확인으로 반영한다. 실제 개선은 아직 미확인이다.
g267 로그인 전 서비스 수정과 만료 없는 v2 승인은 그대로 유지한다.

## B Codex 지시문

기존 g267 지시문의 재부팅은 보류하고 먼저 g268의 일반 연결을 확인해 주세요.
현재 UI 실행 폴더의 BUILD.json은 generation 268,
revision `8a82863f23e6e5d96eab45eaff9242b81233f73a`여야 합니다.
네 EXE와 SHA256SUMS.txt를 다음 독립 해시와 대조해 주세요.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | 0eb248b72472b85cde1388a0cf8be0fb19a3ef9d1f129e8910d60b0f5329db20 |
| krasis-agent.exe | be494fb67ce90cdfa18d9632cf57d6819ff1e4651d33f5671c4ed4adf451d71d |
| krasis-cli.exe | 2e291febcded315743f1451dfb3a6b3c89417616dfe8da3a6c81205d646796c9 |
| krasis-install.exe | a141535b27657f530bc3f49a465086176592dffd91c1c83b73990ee1e0ed3a03 |

같은 폴더 CLI의 `doctor --json`, `status --json`으로 g268 UI·agent·보호 입력
일치, healthy, PROTECTED, A와 CONNECTED를 확인해 주세요. 불일치하면 재설치나
승인 재실행 대신 결과와 새 진단만 보고해 주세요.

제로디가 A의 마우스로 여러 차례 왕복하고 짧은 텍스트 붙여넣기·엔터를 사용한
뒤, 실제 끊김 여부를 확인합니다. Computer Use·UI 도구를 호출하지 않습니다.
그 후 `diagnostics send A --json`과 `diagnostics status --json`에서 SENT를
확인해 주세요. 실제 동작 관측과 CLI 현재 연결 상태를 구분해서 보고합니다.

일반 연결 시험을 마친 뒤 제로디가 직접 B를 한 번 재부팅할 때에만 로그인 전
시험을 이어갑니다. 기존 `g267-prelogin-reboot.md`의 부팅 전·후 서비스
LOGIN-RUNTIME.json 읽기와 진단 절차를 사용하되 버전·해시는 위 g268 기준으로
대체합니다. v2 승인은 다시 만들지 않습니다. 자동 재부팅·서비스 수동 시작·
파일 삭제·보안 설정 변경은 하지 않습니다. PIN 값과 입력 내용은 기록하지 않습니다.
