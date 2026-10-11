# g267 로그인 전 연결 재시험

g266 부팅 시험에서는 A의 입력이 B 로그인 화면에서 작동하지 않았다.
A 기록은 재부팅 뒤 RECONNECTING이며 입력 채널을 다시 맺지 못했다.
B 부팅 서비스의 실제 판단은 g266 진단에 없으므로 원인을 확정하지 않는다.
g267은 WTSConnected 물리 콘솔도 빈 사용자 검증 후 시작 대상으로 인정하며,
서비스 판단을 보호된 고정 파일과 support의 loginRuntime에 남긴다.

## B Codex: 재부팅 전에 한 번 확인

1. 현재 krasis.exe 실행 폴더의 BUILD.json generation 267, revision
   `2b8e7358b37e34f9717d81d3ee5eec4af8cda610`을 확인한다.
   다음 독립 SHA-256 및 SHA256SUMS.txt와 실제 네 EXE를 대조한다.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | cc49a448ac92139fb2603ffca64291b88f612424241b2aab6af7c448205c46b3 |
| krasis-agent.exe | 6cc36d9e84210398f638c51e40ca4447f76e4ca9b4dcc6d161dd8dad3525f405 |
| krasis-cli.exe | 6dabdce2d992d0670ab4d9528b2f830d89850c7a8eb67489562c3b2e8d3a6a60 |
| krasis-install.exe | 83cf3efbe914873411f86622d1e2302f596fde4b4f055e58f7e9b203d1e10c8f |

2. 같은 폴더 CLI의 doctor --json, status --json으로 UI·agent·설치 agent
   g267 일치, healthy, A와 CONNECTED, PROTECTED, 서비스 Running/Automatic을
   확인한다. 기존 LOGIN-GRANT.json이 v2·alias A·expiresAt 없음이며 pending이
   없는지만 확인한다. SID·인증서·키·fingerprint는 출력하지 않는다.
3. `%ProgramFiles%\Krasis\LOGIN-RUNTIME.json`의 startedAt·time·service·console·
   grant·lastWorker·lastFailure만 읽는다. 지금 로그인된 B에서 USER_SESSION은
   정상이며 lastWorker NONE도 부팅 시험 전에는 정상이다. 파일이 없거나 읽지
   못하면 미확인으로 보고한다. 이 파일은 과거 서비스 결정이며 실시간 연결
   상태나 실제 입력 성공의 증거가 아니다.
4. 재승인·설치 반복·서비스 수동 시작·파일 삭제·ACL 변경 없이 결과를 보고한다.
   자동 재부팅이나 Computer Use·UI 도구를 사용하지 않는다.

## 제로디: 실제 시험

A를 켜 둔 채 B만 재부팅한다. B 로그인 화면에서 잠시 기다린 뒤 A의
마우스 이동·클릭·키보드 반응을 확인한다. PIN 값은 기록하거나 전달하지 않는다.
성공/실패와 시험 시각을 알려준다. 실패하면 B 장치로 직접 로그인해 연결을 복구한다.

## B Codex: 시험 뒤 진단

1. 제로디가 B 로그인 화면에서 실제 A 입력 결과를 확인한 뒤 B에 로그인하면,
   doctor/status 및 LOGIN-RUNTIME.json을 다시 읽는다. 이번 부팅의 startedAt이
   시험 전 서비스 시작 시각과 달라졌는지 확인한다. 마지막 실패가 있으면
   time·stage·Win32 숫자와 known만 보고한다. 이전 설치 receipt나 승인 성공을
   이번 입력 성공으로 취급하지 않는다.
2. 같은 g267 CLI로 diagnostics send A --json을 실행하고 diagnostics status --json
   에서 SENT를 확인한다. 새 support의 loginRuntime이 고정 파일과 일치하는지
   확인한다. loginRuntimeIncomplete 또는 부재는 미확인으로 보고한다.
3. 실제 로그인 전 반응·B 수동 개입 여부·부팅 서비스 판단·최종 연결·진단
   전송 결과를 구분해서 답변한다. 실패해도 재부팅·설치·승인을 반복하지 않는다.
