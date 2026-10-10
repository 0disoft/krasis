# B Codex 지시문: 기본 설정 저장 후 로그인 전 승인 확인

B g265의 `startup_read` 실패를 확인했습니다. 설정을 바꾼 적 없는 PC에서
기본 설정 파일이 저장되지 않는 경로를 재현해 g266에서 수정했습니다.
일반 앱 시작이 파일이 없을 때만 기본 설정을 저장하며, 승인기·서비스는
파일을 생성하거나 손상된 설정을 고치지 않습니다. B가 이 경우였는지는
아직 미확인입니다. A에서 g266 업데이트를 전달했습니다.
승인 재시험은 아래 순서로 한 번만 합니다.

1. 현재 `krasis.exe`의 실제 실행 폴더를 `$krasisBundle`로 지정하세요.
   `BUILD.json` generation `266`, revision
   `1a78b344847dc4efbc7710b8129913dc408e1558`를 확인하고,
   네 실제 SHA-256을 아래 독립 값 및 `SHA256SUMS.txt`와 대조하세요.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | 2c57701bf7b91e6302074282a98af95f786b75518aade4bf7337a92792d38f3d |
| krasis-agent.exe | a2d79a76f8ac638546d6aa4279ced656ecb28fce0e692f9a4b2edcddb366f554 |
| krasis-cli.exe | 6f795f384a2a06ed977bfa722f0fdc60c9132d0c42883aad6adf4d2213f765e1 |
| krasis-install.exe | 18dd3c27dabf355cc698c72aadd3a8689fe6edf4f091a17687a4017195f1ccca |

2. 같은 폴더의 CLI로 `version --json`, `doctor --json`, `status --json`을
   확인하세요. UI·agent g266, `healthy`, `PROTECTED`, 서비스 Running/Automatic,
   A와 `CONNECTED` 및 재연결 `SAVED`를 확인합니다. 맞지 않으면 실행하지 말고
   불일치를 보고하세요. 연결 해제·safe-stop·설치 반복은 하지 마세요.
3. 승인 실행 전 UTC 시각과 `%ProgramFiles%\Krasis\LOGIN-GRANT.json` 및
   `LOGIN-GRANT.pending`의 존재 여부를 기록하세요. 기존 승인이나 transaction을
   삭제하지 마세요. 현재 Krasis 사용 계정에서 관리자 확인하여 아래를 한 번만 실행합니다.
   다른 관리자 계정을 선택하지 마세요.

   실행 전에 `%LOCALAPPDATA%\Krasis\launch\settings.json`이 존재하고
   `schema:krasis.app-settings/v1`, `startAtLogin:true`, `autoUpdate`가 실제
   boolean인지 확인하세요. `resume.json`은 실제 저장된 alias가 `A`,
   manualStop이 true가 아니어야 합니다. fingerprint·SID·경로는 답변에
   출력하지 마세요. 파일 부재·손상·꺼짐이면 승인하지 말고 그 항목만 보고하세요.
   설정 토글·파일 편집·ACL 변경으로 우회하지 마세요.

```powershell
$krasisApprovalStart = [DateTimeOffset]::UtcNow
$krasisApproval = Start-Process -FilePath (Join-Path $krasisBundle 'krasis-install.exe') `
    -ArgumentList @('--prepare-login','A') -WorkingDirectory $krasisBundle `
    -Verb RunAs -WindowStyle Hidden -PassThru
$krasisApproval.WaitForExit()
$krasisApproval.Refresh()
$krasisApproval.ExitCode
```

4. `%LOCALAPPDATA%\Krasis\diagnostics\login-configuration-latest.json`를 읽으세요.
   `time`이 이번 시작 시각 이후이고 `action:prepare`인 새 receipt인지 확인합니다.
   `result`, `stage`, `win32Error`, `win32Known`만 보고하세요. stdout 결과 JSON을
   못 받아도 이 receipt를 확인할 수 있습니다. 새 파일이 없으면 미확인으로 보고하고
   이전 파일을 이번 결과로 쓰지 마세요. 파일을 다른 계정 프로필에서 찾지 마세요.
5. 종료 코드 0·receipt result 0·stage complete이면 승인 파일의
   `schemaVersion:krasis.login-grant/v2`, `alias:A`, `expiresAt` 필드 없음과
   pending 없음까지 확인하세요. 하나라도 불일치면 성공으로 보고하지 마세요.
   실패하면 단계와 숫자를 보존하고 재실행·재설치·설정 변경·transaction 삭제·
   ACL 수정으로 우회하지 마세요. 오류 0·known false는 원래 오류 번호 미확인입니다.
6. `doctor --json`, `status --json`을 다시 확인하고 같은 g266 CLI로
   `diagnostics send A --json`을 실행하세요. 전송은 `diagnostics status --json`으로
   SENT를 확인합니다. 새 support에 `loginConfiguration`의 time/result가 이번
   receipt와 일치하는지 확인하세요. 설치 `installation`은 다른 작업의 기록입니다.
7. 버전·파일 검증·이번 승인 종료 코드·receipt의 time/result/stage/Win32·
   승인과 pending 존재·최종 연결·진단 전송을 답변으로 정리하세요.

자동 재부팅, PIN·암호 입력이나 수집, Computer Use·UI 자동화는 하지 마세요.
승인 저장 성공과 실제 재부팅 후 로그인 전 연결 성공은 별도로 보고하세요.
