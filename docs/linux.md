# Linux 개발 빌드

Windows↔Linux 및 Linux↔Linux는 **X11 사용자 세션**을 대상으로 한다.
Windows의 입력·보호 서비스·설치 경로와 기존 mTLS/PIN/리스 프로토콜을 유지한다.
Linux에는 Windows SYSTEM 서비스와 동일한 권한을 부여하지 않는다.

## 지원 범위

| 항목 | Linux |
| --- | --- |
| 키보드·상대 마우스·버튼·휠 | X11 XTEST로 주입, grab으로 로컬 입력 억제 |
| 좌우 경계 전환·귀환 | 각 PC 활성 모니터 하나, 기존 리스·누름 상태 복구 사용 |
| 키보드 배열 | XKB 물리 키 이름을 기존 Windows set-1 코드로 변환 |
| 클립보드 | 명시적으로 켠 peer에 텍스트·PNG 공유; 기존 내용은 켠 것만으로 전송하지 않음 |
| 실행·연결 | CLI, 사용자별 에이전트 자동 시작, XDG 데이터 경로 |
| 잠금·절전·로그아웃 | systemd-logind 감시로 세션 종료·입력 정리 |
| 파일·폴더 클립보드 | Linux 데스크톱 브리지에서 미지원 |
| GUI·보호 입력·로그인 전 입력·peer 자동 업데이트 | Linux 빌드에서 미지원 |
| native Wayland | 미지원. XWayland 접속 성공을 전체 데스크톱 제어로 간주하지 않음 |

Wayland는 compositor가 허용하는 InputCapture/RemoteDesktop portal 및 EIS
입력 경로가 필요하다. X11 grab이나 XTEST를 Wayland 지원으로 대체 보고하지 않는다.
Ubuntu 등에서 이 빌드를 사용할 때는 로그인 화면에서 Xorg/X11 세션을 선택한다.

필수 조건: Go 모듈에 지정된 도구 체인, Linux kernel 5.6 이상(`openat2`),
systemd-logind, 로컬 X11 세션, XTEST·XFIXES 4 이상·XKB 확장, 활성 모니터 하나.
최소 화면 크기는 3×3, 각 축 최대 32767픽셀이다. 루트 실행이나 `/dev/input`,
`/dev/uinput` 권한은 요구하지 않는다. 현재 사용자의 `DISPLAY`와 `XAUTHORITY`를 사용하며
`xhost +`로 접근 제어를 해제하지 않는다. 세션/화면이 바뀌면 다시 시작해야 한다.

## 빌드·시작

```sh
sh scripts/build-linux.sh
cd dist/linux/$(go env GOARCH)
./krasis-cli start
./krasis-cli --json doctor
```

네 실행 파일은 같은 디렉터리에 둔다. `start`/`listen`은 같은 사용자 에이전트를
준비하며, `connect`, `pair listen`/`pair dial`은 필요할 때 자동으로 준비한다.
데이터는 `$XDG_DATA_HOME/Krasis`, 미설정이면 `$HOME/.local/share/Krasis`에 둔다.
같은 UID의 두 에이전트가 다른 HOME으로 단일 인스턴스 잠금을 우회할 수 없다.

PIN 등록과 명시적 양방향 입력 승인은 기존 [CLI 계약](cli/command-contract.md)을 따른다.
등록 후 각 PC에서 상대 주소와 **자신이 나가는 경계**를 설정한다. 예를 들어
Linux가 오른쪽이면 왼쪽 Windows의 상대 경계는 RIGHT, Linux의 상대 경계는 LEFT다.

```sh
./krasis-cli connection set OTHER OTHER_IP:PORT LEFT
./krasis-cli connect OTHER
./krasis-cli clipboard on OTHER
```

클립보드 브리지는 Linux 에이전트가 자동 실행한다. `krasis-clipboard`는 독립
진단 실행용이며, 에이전트 브리지와 동시에 같은 세션에서 실행하지 않는다.
중단은 `disconnect`, 에이전트 종료는 `safe-stop`을 사용한다. 원격 입력 중
Ctrl+Alt+Shift+F12는 로컬 귀환을 요청하며, 대상에 남은 누름 상태는 기존 복구 경로가 해제한다.

## 검증과 한계

원격 CI는 기존 Windows 작업과 별도로 `ubuntu-latest` 작업 하나를 실행한다.
Go 도구 체인은 Windows와 같은 `scripts/runtime/stw-retake.json`에서 선택하며,
Linux 전체 패키지 테스트·`go vet`·배포용 번들 빌드를 검사한다.
`bash scripts/test-linux-x11.sh`는 인증 쿠키가 필요한 Xvfb 두 개를 띄워
입력·클립보드 race 검사와 fixture 프로세스 사이의 인증·경계 왕복을 검증한다.
기존 디스플레이가 있으면 실패하며, 종료·취소 시 서버와 임시 인증 파일을 정리한다.
기본 번호는 `:91`, `:92`이고 `KRASIS_X11_DISPLAY_BASE`로 변경할 수 있다.
CI는 전용 사용자 두 개를 새로 만들고 실제 PAM/logind 세션의 에이전트도 검증한다.
호스트의 로그인 설정을 바꾸는 대신 전용 `krasis-ci` PAM 프로파일을 만들며,
`pam_systemd`가 `Type=x11`, `Class=user`로 등록했는지 실제 logind 속성도 검사한다.
프로파일의 인증 단계는 `pam_deny`를 유지하고 root의 테스트 유닛 생성에만 사용한다.
Linux 복구 자식은 SIGTERM·SIGHUP·SIGINT에 먼저 종료되지 않고 부모 파이프의
EOF까지 저널을 유지한다. 부모가 아직 입력을 보낼 수 있는 동안에는 신호만 보고
해제를 시작하지 않는다. EOF 뒤 해제는 기존 2초 복구 예산을 사용한다.
SIGKILL로 부모와 복구 자식이 함께 제거되는 경우까지 보장하지는 않는다.
각 UID에는 자기 디스플레이의 X11 인증 쿠키만 개인 디렉터리로 전달한다.
`TestLinuxX11ActualCLIInteroperability`는 실제 에이전트 연결을,
`TestLinuxX11ActualLifecycle`는 출발지·수신지 각각의 Lock 요청·LockedHint 변경·
세션 종료를 검사한다. 물리 소스의 키·버튼은 계속 누른 상태로 두어,
일반적인 key-up 전달이 복구 성공으로 오인되지 않도록 한다.
위험 세션의 API 종료, 원격 누름·grab 해제, 같은 신원·신뢰 데이터로 새 PAM 세션에서
재연결하는 것까지 확인한다. CI는 기존 사용자를 재사용하지 않으며,
테스트에 지정한 UID의 PAM 세션만 조작한다.

로컬에서 전용 사용자 두 개를 별도로 준비한 뒤 다음처럼 실행할 수 있다.
`sudo -n`, 실제 systemd-logind, Xvfb·xauth·busctl, 별도로 준비한 그래픽 테스트용
PAM 프로파일이 필요하다. CI의 프로파일 내용은 워크플로에서 확인할 수 있다.

```sh
KRASIS_X11_ACTUAL_LIFECYCLE=1 \
KRASIS_X11_TEST_LOCAL_USER=DEDICATED_A \
KRASIS_X11_TEST_PEER_USER=DEDICATED_B \
KRASIS_X11_TEST_PAM_SERVICE=krasis-ci \
bash scripts/test-linux-x11.sh
```

`KRASIS_X11_TEST_SCOPE=logind`는 실제 세션 테스트만 실행한다. CI는 기본 `all`로
입력·클립보드 검사도 유지한다. 이 검사는 실제 화면 잠금 UI·시스템 절전·전원 종료나
물리 기기 검증을 대체하지 않는다.

2026-11-11에 Debian 13 가상 머신과 격리된 Xvfb 두 개에서 다음을 확인했다.

- 실제 에이전트·CLI를 서로 다른 두 UID와 PAM/logind 세션에서 실행하고,
  PIN 등록·mTLS 연결·경계 진입·상대 이동·키·버튼·양방향 휠·귀환을 확인했다.
  키와 버튼을 누른 채 연결을 종료한 뒤 해제되는 것과 양쪽 `LOCAL` 복귀도 확인했다.
- Linux 파일 보안·실행·CLI·세션 감시·peer 테스트와 입력·클립보드 race 검사,
  Linux/Windows 관련 패키지의 `go vet`, Windows 입력·agent·CLI 회귀 테스트를 통과했다.
- 네 Linux 명령을 CGO 없이 빌드하고 `krasis-linux-amd64.tar.gz`로 묶었다.

물리 Windows↔Linux 조합, 실제 X11 window manager의 팝업/grab 경합,
실제 시스템 절전·전원 종료와 화면 잠금 UI는 이번 환경에서 실행하지 않았다.
잠금·비활성화·절전·종료의 이벤트 분기는 단위 테스트로 검증했다.

일반 단위 테스트는 입력을 만들지 않는다. 다음 smoke는 **격리된 Xvfb**에서만
실행한다. 실제 작업 중인 X11 화면에 적용하면 키·마우스가 움직인다.

```sh
DISPLAY=:91 KRASIS_X11_TEST=1 go test -race -count=1 ./internal/x11desktop ./internal/wininput
DISPLAY=:91 KRASIS_X11_TEST=1 go test -count=1 -timeout=3m ./internal/residentagent -run '^TestLinuxX11CLIInteroperability$' -v
```

Xvfb 검증은 실제 compositor·로그인 화면·물리 장비 조합의 검증을 대체하지 않는다.
Linux 포인터 감시는 4ms 주기로 경계 위치를 확인한다. 지원하지 않는 물리 키,
키맵/화면 변경, grab 충돌, X 서버 통신 실패는 조용히 무시하거나 재주입하지 않고
기존 세션 실패·정리 경로로 전달한다. 파일 접근은 dirfd와 `openat2`를 사용하며,
커널 지원이 없으면 경로 기반 대체 구현으로 보안을 낮추지 않는다.
