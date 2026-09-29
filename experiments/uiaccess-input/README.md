# UIAccess PIN 입력 시험

이 시험물은 B에서만 실행합니다. 네트워크 연결이나 Krasis 세션 없이 전면의
`CredentialUIBroker.exe`를 확인하고 Esc 누름·뗌을 한 번만 보냅니다. PIN, 창 텍스트,
키 입력, 화면을 읽거나 기록하지 않습니다. API 반환과 창이 사라진 것만으로 성공을
판정하지 말고 B 화면에서 직접 확인해야 합니다.

Windows가 UIAccess를 부여하려면 EXE에 `uiAccess=true` 매니페스트와 신뢰된
코드 서명이 있어야 하고, 관리자만 쓸 수 있는 `Program Files` 아래에 설치해야
합니다. 개발용 서명 인증서를 B의 신뢰 저장소에 추가하는 것은 **시험용**이며,
제품 자동 업데이트가 이를 몰래 설치해서는 안 됩니다. 시험 후 인증서와 EXE를
제거하세요. 서명되지 않았거나 설치 조건을 충족하지 못하면 프로그램은
`uiaccess_unavailable`을 표시하고 입력을 보내지 않습니다.

`dialog_dismissed_candidate`가 나와도 실제 PIN 창이 닫혔는지 B에서 확인하세요.
`dialog_still_open`이면 UIAccess만으로는 이 PC의 PIN 창을 제어할 수 없으므로
기존 사용자 모드 에이전트의 보호창 연결 해제 동작을 유지합니다. 이 시험은
관리자 PowerShell, UAC 보안 데스크톱, 로그인 화면 또는 연결 중 원격 입력을
검증하지 않습니다.

개발 PC에서 `scripts/build-uiaccess-input-probe.ps1`로 빌드하고
`scripts/sign-uiaccess-input-probe-test.ps1`로 임시 인증서에 서명합니다.
g91 배포본은 g90 수신기를 통해 기기 업데이트하면 시험 파일 여섯 개를
B의 새 `krasis.exe`와 같은 폴더로 전달합니다. g89 수신기에는 먼저
시험 파일이 없는 g90 전환 버전을 업데이트한 다음 g91을 보내야 합니다.
파일 전달은 인증서 신뢰 등록이나 시험 실행을 하지 않습니다.
B의 관리자 PowerShell에서 새 업데이트 폴더의
`install-test.ps1 -ArtifactDirectory <결과 폴더> -ExpectedSHA256 <A에서 별도로 확인한 EXE 해시> -ExpectedThumbprint <A에서 별도로 확인한 인증서 지문>`을 실행합니다. 결과 폴더 안의
`TEST-CERT-DETAILS.txt`만 믿고 지문을 결정하지 마세요. 관리자 창을
닫고 일반 탐색기에서 `C:\Program Files\KrasisUIAccessProbe\KrasisUIAccessProbe.exe`를
실행해 PIN 창 시험을 합니다. 결과 코드와 **실제로 PIN 창이 닫혔는지** 기록합니다.
시험 뒤 B의 관리자 PowerShell에서 `remove-test.ps1 -Thumbprint <TEST-CERT-DETAILS.txt의 지문>`을
실행해 설치 파일과 신뢰 인증서를 제거합니다. 두 스크립트는 다른 버전의 파일이나
기존 신뢰 지문을 건드리지 않도록 일치 여부를 확인합니다.
