# SYSTEM 입력 시험 — B Codex 작업 지시문

g98에서 UIAccess·입력 데스크톱 연결만으로 PIN 창을 닫지 못했습니다.
이 시험은 관리자 설치한 **일회성 LocalSystem 서비스**가 현재 콘솔 세션에 SYSTEM·UIAccess
시험 프로세스를 띄우는 경로를 비교합니다. 서비스와 입력 프로세스는 동일한 EXE입니다.
g99는 서비스 시작이 오류 740으로 실패해 입력을 시험하지 못했습니다. 다음 시험물은
서비스 실행 매니페스트를 `asInvoker / uiAccess=false`로 수정합니다. 자식 토큰의 UIAccess는
서비스가 명시적으로 설정하고, 입력 직전에 실제 부여 여부를 다시 확인합니다.
기존 `krasis.exe`·에이전트는 일반 권한을 유지합니다. 시험 자체에는 원격 연결이 없습니다.

g100은 서비스 시작에 성공했지만 Esc 수락이 0개, Win32 오류가 5였고 PIN 창이 유지됐습니다.
g101은 서비스 자신의 토큰 대신 Deskflow처럼 현재 콘솔 세션의
`System32\winlogon.exe` 토큰을 복제합니다. 토큰의 SYSTEM 신원과 세션을 확인하며,
원래 세션을 변경하지 않습니다. 이 경로도 B에서 UIAccess·세션·데스크톱 검사를 통과했지만,
Esc 수락이 0개, 오류 5였고 PIN 창이 유지됐습니다.
g102는 같은 토큰 경로를 유지하면서 데스크톱 메시지 큐와 **절대 표시·활성화하지 않는 창**을
만들고, 자체 메시지 처리 중 Esc 누름·뗌을 한 이벤트씩 전송합니다.
토큰 선택이나 창 준비가 실패하면 이전 경로로 대체하지 않습니다.

g103 계측 시험은 같은 입력 경로를 유지하고 실제 입력 순간의 무결성 RID·상승 여부를
추가로 확인합니다. Deskflow의 같은 PIN 창 제어는 Computer Use 없는 재시험에서
System RID 16384·상승 true·UIAccess true 조건으로 성공했습니다. 기존 g102 자식의
무결성 RID는 미측정이므로 아직 실패 원인을 단정하지 않습니다.

g103 재시험에서는 입력 순간도 System RID 16384·상승 1·UIAccess 1·세션 1이었지만
Esc 수락은 0개, 오류는 5였습니다. Computer Use·UI 도구도 호출하지 않았습니다.
다음 시험은 토큰과 키 이벤트를 유지하고 데스크톱 핸들의 요청 권한에
`DESKTOP_JOURNALPLAYBACK` 한 비트만 추가합니다. 이는 Deskflow의 `GENERIC_WRITE`에
포함되지만 g103에는 빠진 권한입니다. 시스템 보안 설정이나 데스크톱 ACL을 변경하지
않으며, journaling hook을 설치하지도 않습니다. 아직 실제 성공은 미검증입니다.

**시험이 끝날 때까지 Computer Use·UI 자동화·화면 캡처·창 텍스트 조회를 호출하지 마세요.**
PIN 창과 Esc 반응은 제로디가 직접 확인합니다. 결과 JSON과 서비스 상태는 터미널로만
조회합니다. 서비스 시작 전에 안내를 모두 읽고, 실제 반응 확인 후 결과를 읽으세요.

g103 기기 업데이트는 기존 수신기와 호환되는 여섯 파일 묶음으로 비교 시험을 전달합니다.
EXE 파일명은 수신기 호환을 위해 `KrasisUIAccessProbe-TESTSIGNED.exe`를 유지합니다.
g98의 일반 사용자 시험과 실행·설치 방법이 다르므로 이 안내문을 따르세요.
g103 EXE SHA-256은 `b7e6e3ecb6e4d63060849a3b3fe4c42ae4dd4d0817aa25c3b783cbf375a42863`,
인증서 지문은 `F0417926F4B338DA41484779960350BFC064C543`입니다.
만료는 2026-10-07 12:58:48 UTC입니다. B 설치 전 A의 별도 전달값과 대조하세요.

시험은 실제 전면 `CredentialUIBroker.exe`와 같은 세션의 Default 데스크톱만 확인하고
Esc 한 번을 보냅니다. PIN·창 텍스트·클립보드·화면을 읽거나 기록하지 않습니다.
SYSTEM 프로세스는 창을 표시·활성화하지 않습니다. 준비된 숨은 창은 단일 입력 메시지 처리 후
데스크톱을 복구하기 전에 제거합니다. 결과는 고정된 코드와 숫자만 JSON으로 저장합니다.

## B에서 실행

1. 실행 중인 Krasis의 실제 업데이트 폴더에서 시험 EXE·공개 인증서·지문 기록·이 안내문·
   `install-test.ps1`·`remove-test.ps1` 여섯 파일을 확인합니다.
   **A가 별도로 전달한 EXE SHA-256과 인증서 지문**을 파일 및 서명자와 대조합니다.
   폴더 안의 지문 기록만 믿고 인증서를 설치하지 마세요.
2. B의 현재 콘솔 사용자로 관리자 PowerShell을 열고, 스크립트 두 개를 읽은 뒤 실행합니다.

   ```powershell
   & (Join-Path $artifactDirectory 'install-test.ps1') -ArtifactDirectory $artifactDirectory `
     -ExpectedSHA256 '<A의 EXE SHA-256>' -ExpectedThumbprint '<A의 인증서 지문>'
   ```

   EXE는 `C:\Program Files\KrasisSystemInputTrial`에 설치됩니다.
   설치 스크립트는 서비스나 입력 시험을 시작하지 않습니다. 설치 후 서명 `Valid`를 확인합니다.
3. PIN 창을 띄울 준비를 하고 B에서 로컬로 다음 명령을 한 번 실행합니다.

   ```powershell
   Start-Service KrasisSystemInputTrial
   ```

   서비스 시작이 오류로 끝나면 즉시 오류 코드와 서비스 상태를 확인하고 중단하세요.
   프로세스·실행 표시 파일·결과 파일이 모두 없으면 입력 시험 단계에 도달하지 않은 것입니다.
   이 경우 130초를 기다리거나 PIN 입력 효과를 추정하지 않습니다.

   **2분 안에** B에서 Chrome 자동완성을 눌러 Windows 보안 PIN 창을 띄우세요.
   PIN을 입력하지 마세요. 시험 Esc가 창을 닫았는지 현장에서 확인합니다.
4. `Program Files\KrasisSystemInputTrial\result.json`과 `service-result.json`을 읽습니다.
   코드, `system`, `consoleSession`, `desktopBound`, `alreadyDefault`, `inputAttempted`,
   `accepted`, `win32Error`, `releaseAccepted`, `tokenInspected`, `uiAccess`,
   `tokenSessionMatches`, `winlogonTokenLaunched`, `queueReady`, `hiddenWindowCreated`,
   `messageHandled`, `downAccepted`, `upAccepted`, 실제 PIN 창 반응을 보고합니다.
   `accepted`는 누름·뗌 두 호출의 수락 합계이며, `releaseAccepted`는 뗌 실패 후 추가 정리 호출입니다.
   `winlogonTokenLaunched`는 서비스 결과에서만 자식 생성 성공을 뜻합니다.
   서비스의 `uiAccess`·데스크톱·입력 값은 서비스 자신의 상태입니다.
   입력 자식의 실제 값은 `result.json`에서 확인하며, 서비스 값과 합치지 않습니다.
   새 계측의 `reportProcessToken`은 결과를 쓰는 순간의 프로세스 상태입니다.
   `inputTokenCaptured`, `inputProcessToken`, `inputEffectiveToken`,
   `inputEffectiveTokenSource`는 입력 callback에서 읽은 값입니다. callback 계측까지
   도달하지 못하면 `inputTokenCaptured=false`이며 입력 토큰 필드는 null입니다.
   계측 후 대상 재확인에서 중단되면 captured=true여도 inputAttempted=false일 수 있습니다.
   각 토큰의 `integrityRid`, `elevated`, `elevationType`, `uiAccess`, `session`,
   `restricted`, `appContainer`, `queryError`를 보고하세요. 상태 값 0/1은 false/true이며,
   null은 조회되지 않았거나 실패한 값입니다. `queryError`는 첫 조회 오류 번호입니다.
   유효 토큰 source가 `process`이면 스레드 impersonation 토큰이 없어서 프로세스
   토큰을 사용한 것이고, `thread`이면 그 스레드 토큰을 읽은 것입니다. 오류 시에는
   `unknown`으로 남깁니다. 특히 `result.json`의 입력 순간 RID를 확인하세요.
   `configuredDesktopAccessMask`도 보고하세요. 다음 시험의 값은 171(0xAB)이며
   g103 요청값 139(0x8B)에 입력 재생 접근 비트 32만 더한 값입니다. 이 필드는
   컴파일된 요청 설정이며 서비스가 데스크톱에서 입력했다는 뜻이 아닙니다.
   `code=0`은 닫힘 후보이며 실제 화면 확인이 필요합니다. `4`는 입력 수락 실패,
   `5`는 창이 계속 열려 있음, `2`는 UIAccess 없음, `3`은 대상 미관측입니다.
   서비스 코드 `20~31`이면 세션·토큰·생성·중단·시간 초과 단계에서 실패한 것입니다.
   `32`는 고정된 winlogon 토큰의 선택·검증·복제 실패입니다.
   `33~37`은 메시지 창 등록·생성·전송·대기·제거 단계의 실패입니다.
   Win32 오류 번호만으로 UIPI 등 특정 원인을 확정하지 마세요.
   입력 결과 파일이 없으면 서비스 결과와 파일 부재를 그대로 보고합니다.
5. 결과를 확인한 뒤에만 필요하면 B 터치패드로 PIN 창을 닫습니다.
   수동 닫힘 시점을 결과와 구분합니다. 관리자 PowerShell에서 제거합니다.

   ```powershell
   & (Join-Path $artifactDirectory 'remove-test.ps1') -Thumbprint '<A의 인증서 지문>'
   ```

   서비스·해당 경로의 프로세스·설치 폴더와 Root·TrustedPublisher의 해당 지문이
   사라졌는지 확인합니다. 결과 JSON은 제거 전에 읽고 필요한 숫자만 보고하세요.
   Krasis 본체·다른 인증서·다른 서비스를 제거하지 않습니다.

## 범위와 제한

서비스는 수동 시작이며 자동 재시작·시작 프로그램 등록이 없습니다. 한 설치에서 한 번만
실행할 수 있습니다. 대상 대기는 120초, 서비스가 소유한 자식의 전체 수명은 130초입니다.
서비스 종료·크래시·세션/사용자 변경·시간 초과 때 생성 당시부터 소유한 Job의 자식을 종료합니다.
서비스 설정·파일 수정·시작·중지는 관리자 또는 SYSTEM만 허용합니다.
다른 실행 파일·명령·세션·PID를 넘기는 입력이나 원격 IPC는 없습니다.

이 시험의 성공은 로컬 Esc 경로의 증거입니다. A→B 마우스·키보드, 관리자 PowerShell,
실제 PIN 입력, 제품 업데이트 중 복구는 별도 검증이 필요합니다. Windows 10 이상만 대상으로 합니다.
