# B Codex 지시문: g262 로그인 전 연결 승인

B 노트북에서 Krasis g262의 로그인 전 연결 승인을 저장해 주세요.
제품 소스코드가 없어도 실행 중인 앱과 같은 폴더의 CLI·설치기로 진행할 수 있습니다.
이 지시문의 기준은 g262이며, 다른 버전에는 적용하지 마세요.

A는 g262 앱·보호 입력 갱신과 상대 별칭 B에 대한 승인 저장을 완료했습니다.
A에서 B로 g262 자동 업데이트도 보냈습니다. B는 자신의 실제 상태를 확인하세요.
실제 재부팅 시험은 아직 하지 않았습니다.

## 1. 실제 배포·설치 확인

- 실행 중인 `krasis.exe`의 전체 경로를 조회하고 그 폴더를 `$krasisBundle`로 지정하세요.
  오래된 `dist` 폴더나 복구 CLI의 폴더를 임의로 선택하지 마세요.
- 같은 폴더의 `BUILD.json`에서 generation `262`, revision
  `f9c967ada38e418ccd5b542b549e6216b34e2322`를 확인하세요.
- 네 파일의 실제 SHA-256을 아래 독립 값 및 `SHA256SUMS.txt`와 대조하세요.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | ad8736fa822058daa38616b288074ff716d73ef1593d14b36e4854c878f3560a |
| krasis-agent.exe | fa902535ed2bc8a5d3168ce6639b303df51ec1e9fd573d2add52d68c663ac9d7 |
| krasis-cli.exe | 0b3003cfeab183d75e5d2690f7a0d5e95596b90f49712a5a59a0131e3ede5fe8 |
| krasis-install.exe | c4481ccd77f3f1994723b73bbe703a7b86386026d943309d379e30c746bbc828 |

```powershell
& (Join-Path $krasisBundle 'krasis-cli.exe') version --json
& (Join-Path $krasisBundle 'krasis-cli.exe') doctor --json
& (Join-Path $krasisBundle 'krasis-cli.exe') status --json
Get-Service KrasisInput | Select-Object Name,Status,StartType
```

CLI·UI·실제 agent가 모두 g262인지 확인하세요.
`doctor`는 `healthy`, `installedAgent.state:VERIFIED`, `service.state:RUNNING`,
`service.pathStatus:MATCHED`, `agentLocation:PROTECTED`, `protectedInput:CONFIGURED`여야 합니다.
서비스의 시작 유형은 `Automatic`, 연결은 `CONNECTED`, 상대 별칭은 `A`여야 합니다.
설치된 `%ProgramFiles%\Krasis\Input\krasis-agent.exe` 해시도 위 agent 값과
대조하고, input 이미지 해시는 같은 폴더의 `INPUT-INSTALL.json`과 대조하세요.

다르면 승인을 진행하지 말고 불일치와 진단을 보고하세요. 이미 설치가 맞으면
`--install`을 반복하지 마세요. Windows 시작 시 실행 옵션이 켜져 있어야 합니다.
연결 해제나 safe-stop은 저장된 재연결을 중단하므로 승인 전에 실행하지 마세요.

## 2. 관리자 확인으로 한 번 승인

현재 Krasis를 사용하는 동일 Windows 계정에서 다음 명령을 실행하세요.
UAC는 제로디가 직접 확인하고, 다른 관리자 계정으로 바꿔 실행하지 마세요.

```powershell
$krasisApproval = Start-Process -FilePath (Join-Path $krasisBundle 'krasis-install.exe') `
    -ArgumentList @('--prepare-login','A') -WorkingDirectory $krasisBundle `
    -Verb RunAs -WindowStyle Hidden -PassThru
$krasisApproval.WaitForExit()
$krasisApproval.Refresh()
$krasisApproval.ExitCode
```

종료 코드 `0`을 확인하세요. stdout을 확보할 수 있다면
`krasis.login-configuration-result/v1`, `code:0`, `state:configured`도 확인하세요.
위 방식으로 stdout을 확보하지 못했다면 JSON은 미확인으로 보고하세요.

`%ProgramFiles%\Krasis\LOGIN-GRANT.json`에서 schemaVersion, alias `A`,
createdAt, expiresAt, allowInboundControl을 읽고 유효 기간과 승인 대상을 확인하세요.
`LOGIN-GRANT.pending`이 없어야 합니다. 전체 SID·인증서·개인키는 출력하지 마세요.
서비스 Running/Automatic과 `doctor`, `status`를 다시 확인하세요.
승인 파일 저장은 로그인 전 연결의 실기기 성공으로 보고하지 마세요.

## 3. 진단 전송·후속 시험

```powershell
& (Join-Path $krasisBundle 'krasis-cli.exe') diagnostics send A --json
```

`QUEUED`와 `SENT`를 구분하고, 필요하면 `diagnostics status --json`으로 확인하세요.
generation/revision·해시 대조·설치 상태·승인 종료 코드·승인 만료·최종 연결·진단
전송 결과를 A에 전달할 답변으로 정리하세요. 실패하면 단계와 확보한 오류를 보존하세요.
승인 실패 시 반복 실행, transaction 삭제, 레지스트리/ACL/보안 설정 변경은 하지 마세요.

자동 재부팅은 하지 마세요. 양쪽 승인 확인 후 제로디가 B를 재부팅하고 Windows
로그인 화면에 둔 상태에서 A의 마우스 이동과 PIN 값이 아닌 Esc 등으로 반응을
확인합니다. 로그인 후 일반 연결·입력 복귀도 확인합니다. 이어서 A 재부팅 시험은
별도로 진행합니다. PIN·암호를 입력하거나 읽거나 수집하지 말고 Computer Use·
UI 자동화는 호출하지 마세요. 승인을 되돌릴 때는 관리자 확인으로 제품 설치기의
`--disable-login`을 사용합니다.
