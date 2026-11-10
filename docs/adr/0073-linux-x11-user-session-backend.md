# ADR 0073: Linux X11 사용자 세션 백엔드

- 상태: Accepted (개발 빌드)
- 날짜: 2026-11-11

## 결정

Linux에서 기존 인증·권한·세션 모델을 재사용하고 플랫폼 I/O만 추가한다.
Windows 파일은 입력 오류 분류 상수의 플랫폼 공통화 외에 동작을 바꾸지 않는다.
기존 외부 필수 함수 시그니처와 wire protocol은 유지한다.

- `internal/x11desktop`: 로컬 Unix 소켓, Xauthority, 750ms 작업 예산,
  XKB 물리 키 매핑, XTEST 주입, XFIXES 커서, X11 grab 수명.
- `internal/wininput/*_linux.go`: 기존 InputAdapter/SourceHook 계약 구현.
  native owner의 화면 정보를 재사용해 매 이동 프레임의 재접속·재인증을 방지한다.
- `internal/residentagent/portable_input_linux.go`: 별도 crashguard 프로세스가
  주입 전 누름 상태를 기록하고 부모 EOF에 독립적으로 해제한다.
- `internal/securefs/*_linux.go`: dirfd/openat2/no-follow/no-replace, 파일 수명과
  Close 동기화. namespace 변경은 신뢰할 수 없는 쓰기 주체가 없는 개인 디렉터리에서만 수행한다.
- Linux 사용자별 flock, XDG 경로, 독립 프로세스 실행 및 CLI 시작 제공.
- systemd-logind는 세션 잠금·비활성화·절전·종료를 기존 lifecycle 이벤트로 전달한다.
- Linux clipboard는 별도 X11 연결을 사용하며 기존 로컬 API의 peer 승인·클립보드 opt-in을 따른다.

## 제외와 검증

native Wayland, 관리자/로그인 전 입력, Linux GUI, 파일 클립보드 및 Linux 자동
업데이트는 이 백엔드에 포함하지 않는다. XWayland가 존재해도 Wayland를 지원한다고
보고하지 않는다. 모든 Linux 배포판 지원이나 물리 Windows↔Linux 왕복 성능을
Xvfb/교차 컴파일 결과만으로 보장하지 않는다.

Go X11 protocol 구현 `github.com/jezek/xgb`의 v1 안정 버전을 사용한다. CGO나
X11 개발 헤더가 필요 없고, 라이선스는 모듈의 BSD/특허 고지를 따른다. 버전 변경은
Go 모듈 합계와 검증으로 추적한다. 기존 Windows 의존성·프로토콜·CI runner는 변경하지 않는다.
