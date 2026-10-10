# B Codex 지시문: 로그인 전 승인을 반복 만료 없이 전환

B의 Krasis g264 업데이트·보호 입력 상태를 확인하고, 기존 기한 있는 승인을
새 영구 승인으로 한 번 전환해 주세요. A에서 g264 묶음을 이미 전달했습니다.
제품 소스코드 없이 실행 중인 앱 폴더의 CLI와 설치기로 진행할 수 있습니다.

1. 실행 중인 `krasis.exe` 전체 경로의 폴더를 `$krasisBundle`로 지정하세요.
   `BUILD.json`은 generation `264`, revision
   `d5101d7803707297873399b5cc0ddb03a44e3dfa`여야 합니다.
2. 네 실제 EXE 해시를 아래 독립 값과 `SHA256SUMS.txt`에 대조하세요.

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | feb16d75e70aedeb28fde87ba8cc9e04783f6b32b8ede4088c5eebc597f70152 |
| krasis-agent.exe | 35da4afe1de89ea19dc01f21f36061265ad813085f848e6f9dfee289a9c0bbb6 |
| krasis-cli.exe | 330c47a85ff7be0915123f25062186c9f10bc8b2df001ec99a733a6c4d9258c5 |
| krasis-install.exe | b61f637546bcbede2f1445905b01398ba210c1770e1fbb484057198ae5f30844 |

3. 같은 폴더의 CLI로 `version --json`, `doctor --json`, `status --json`을
   확인하세요. UI·agent가 g264, `healthy`, `PROTECTED`, 서비스 Running/Automatic,
   설치 agent 해시 일치, A와 `CONNECTED`여야 합니다. 맞지 않으면 승인 전에
   중단하고 실제 불일치와 진단을 보고하세요. 설치를 반복하지 마세요.
4. 현재 Krasis 사용 계정에서 아래를 한 번 실행하세요. UAC는 제로디가
   직접 확인합니다. 연결 해제·safe-stop·다른 관리자 계정 선택은 하지 마세요.

```powershell
$krasisApproval = Start-Process -FilePath (Join-Path $krasisBundle 'krasis-install.exe') `
    -ArgumentList @('--prepare-login','A') -WorkingDirectory $krasisBundle `
    -Verb RunAs -WindowStyle Hidden -PassThru
$krasisApproval.WaitForExit()
$krasisApproval.Refresh()
$krasisApproval.ExitCode
```

5. 종료 코드 `0`을 확인하세요. `%ProgramFiles%\Krasis\LOGIN-GRANT.json`은
   `schemaVersion:krasis.login-grant/v2`, `alias:A`, `expiresAt` 필드 없음이어야
   합니다. `LOGIN-GRANT.pending`도 없어야 합니다. 전체 SID·인증서·개인키를
   출력하거나 수집하지 마세요. stdout을 못 읽었다면 결과 JSON은 미확인으로
   구분하고, 종료 코드와 승인 파일 확인 결과로 보고하세요.
6. `credential show CONNECTOR --json`으로 `automaticRenewal:true`,
   `expired:false`를 확인하세요. 반대 역할로 페어링했다면 LISTENER를 사용하세요.
   일반 인증서 갱신에 `credential renew`를 실행하지 마세요. 그 명령은 새 역할
   키를 만들므로 다시 페어링해야 합니다.
7. `doctor --json`과 `status --json`을 다시 확인하고
   `diagnostics send A --json`으로 새 진단을 전송하세요. QUEUED/SENT를
   구분하고 필요하면 `diagnostics status --json`을 조회하세요.
8. generation/revision·파일 검사·서비스·승인 종료 코드·v2와 만료 없음·현재
   연결·진단 전송 결과를 답변으로 정리하세요. 실패하면 단계와 확보한 오류를
   보존하고, transaction 삭제·ACL/레지스트리/인증서 저장소 변경은 하지 마세요.

자동 재부팅하지 마세요. 이후 제로디가 B를 재부팅해서 Windows 로그인 화면에
둔 상태의 A→B 반응과 로그인 후 일반 연결 복귀를 확인합니다. PIN·암호 입력,
열람·기록과 Computer Use·UI 자동화는 하지 마세요. 실제 부팅 연결 시험 전에는
승인 저장 성공과 부팅 연결 성공을 구분하세요.

이번은 기한 있는 g262 승인을 전환하는 한 번의 작업입니다. g264 v2 승인은
사용자 해제까지 유지되고, 정상 인증서 갱신·버전 업데이트에는 재승인하지 않습니다.
자동 시작 해제·수동 연결 해제·신뢰 철회·키 교체는 연결을 중단합니다.
