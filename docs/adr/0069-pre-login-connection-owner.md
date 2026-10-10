# ADR 0069: 로그인 전 연결의 서비스 소유권

- 상태: Accepted for lifetime gate and grant provisioning; runtime integration pending
- 결정일: 2026-10-10
- 관련: ADR 0036, ADR 0038, ADR 0064, ADR 0066, ADR 0068

## 문제

현재 Windows 자동 시작은 HKCU Run의 사용자 로그인 자동 시작이다. 부팅 시
자동 시작하는 `KrasisInput` 서비스는 보호 입력만 담당하며 peer 연결을 만들지
않는다. 로그인 뒤의 Chrome PIN 창 제어 성공과 재부팅 직후 LogonUI 입력은
서로 다른 수용 기준이다.

[Deskflow Windows watchdog](https://github.com/deskflow/deskflow/blob/master/src/lib/platform/MSWindowsWatchdog.cpp)은
서비스가 활성 세션의 winlogon 토큰으로 core를 실행하고 세션 변경 때 다시
관리하는 구조를 사용한다. Krasis도 GUI를 로그인 전에 띄우는 대신 서비스가
제한된 연결 에이전트를 소유하는 방향으로 확장한다.

## 수명 관리 구현

`internal/loginagent`는 활성 물리 콘솔의 사용자 존재 여부를 읽고 별도의
서비스 연결 owner를 관리하는 기반이다. WTS 조회는 고정 세션·정보 종류만
사용하고 반환 버퍼를 해제한다. 사용자 이름은 빈 값 여부만 판단하고 로그,
진단 또는 반환 구조에 담지 않는다. API 조회 실패를 로그아웃으로 간주하지
않는다. 사용자 잠금과 UAC는 사용자 세션이며 새 로그인 전 owner를 시작하지 않는다.

owner는 검증된 기계별 grant 식별자와 로그인 전 콘솔이 모두 있을 때만 시작을
요청한다. 이 식별자 자체는 실행 또는 입력 권한이 아니다. 실제 시작 backend는
고정 설치 이미지·기계별 승인·콘솔·자식 토큰을 별도로 검증해야 한다.

로그인, 콘솔 변경, grant 변경·해제, 관측 실패, 호출 취소는 기존 worker의
수명을 취소한다. 취소되지 않은 5초 정리 문맥으로 Stop을 실행하고 입력·연결
정리가 끝난 Done을 확인한 뒤에만 다음 worker를 허용한다. 정리 실패는 owner를
영구 차단한다. 시작 실패 또는 예기치 않은 worker 종료는 같은 grant/콘솔의
폴링으로 재실행하지 않는다. 시작 전후 콘솔을 다시 읽어 시작 중 로그인도 처리한다.

## 기계별 승인과 현재 정책 읽기

관리자 권한의 `krasis-install --prepare-login PEER`는 실행 계정의 현재
재연결 대상으로 저장된 한 기기만 선택한다. 사용자·상대 역할·기기 및 transport
인증서 지문·입력 허용 상한·인증서보다 길지 않은 만료를 고정 승인 기록에 담는다.
기록에는 개인키·실행 파일·명령·경로가 없고, 설치 버전 지문도 넣지 않는다.
따라서 코드 업데이트만으로 재승인이 필요해지지는 않는다.

파일은 기존 설치 잠금을 얻은 뒤 `%ProgramFiles%/Krasis/LOGIN-GRANT.json`에
보호된 pending 파일의 쓰기·동기화·닫기와 원자적 이동으로 저장한다. 기존 보호
입력이 설치되어 있어야 하며 설치 transaction이 남아 있으면 보류한다.
파일의 관리자/SYSTEM 소유·DACL·실제 핸들·reparse/hardlink 검증을 재사용한다.
해제 명령은 동일 잠금 아래 해당 기록만 삭제하고 부재를 확인한다.
쓰기 이후 검증·닫기 결과가 불명확하면 성공으로 표시하지 않는다.
부팅 조회에서 pending 파일이 남아 있으면 이전 승인 기록도 사용하지 않는다.
명시적 설정 해제는 승인 기록과 이 pending 파일만 제거하고 양쪽 부재를 확인한다.

서비스의 프로필 위치는 보호된 Windows ProfileList에서 결정한다. 최초 준비는
현재 계정의 KnownFolder LocalAppData와 일치해야 하며, 리디렉션된 LocalAppData는
현재 지원하지 않는다. 원래 사용자/SYSTEM 전용 DACL을 읽기 전용으로 검증한다.
일반 호출자는 자기 SID만 고를 수 있고, SYSTEM은 보호된 승인 기록에서 고른 SID만
사용해야 한다. 현재 사용자 검증의 정책을 바꾸거나 다른 사용자에게 접근 권한을
부여하지 않는다.

원래 identity·trust 잠금 안에서 기존 인증서와 신뢰 정보를 읽는다. 인증서 생성,
갱신, ACL 수정과 페어링 복원은 하지 않는다. 설정·재연결 파일이 없거나 손상됐으면
기본값으로 허용하지 않는다. 수동 해제·자동 시작 해제·별칭/지문/역할 변경·신뢰
철회·키 교체·만료는 부팅 연결을 중단시키며 현재 inbound 허용을 기록의 상한과
교차한다. 이 검사는 실제 runner가 연결·lease 승인과 관측 루프에 연결해야 한다.

설정 준비 CLI는 `prepared_runtime_pending`, `active:false`를 반환한다.
현재 사용자 UI에는 아직 로그인 전 연결 옵션을 노출하지 않는다.

## 로그인 전 실행 primitive

서비스 전용 `launchLoginConsole`은 고정 설치·SCM 실행 여부·현재 물리 콘솔·
보호된 승인 기록·원래 사용자 정책을 시작 전과 자식 resume 직전에 다시 읽는다.
일반 worker는 계속 `--input-worker` / `winsta0\default`를 사용한다. 로그인 전
worker는 별도 고정 모드 `--login-worker` / `winsta0\Winlogon`만 선택할 수 있고
IPC로 실행 경로·명령·데스크톱을 선택할 수 없다. 아직 child entry point와
서비스 루프에 연결하지 않아 현재 배포에서는 이 모드를 실행하지 않는다.

두 경로 모두 자식을 suspended 상태로 생성하고 생성 시 Job에 넣는다. 자식의
실제 SYSTEM·UIAccess·System 무결성·세션·이미지를 확인한 뒤 resume한다.
시작 실패는 소유 Job을 종료하고 실제 프로세스 종료를 기다린다. 종료 또는
핸들 정리가 미확인이면 recovery 오류를 보존한다. `ConsoleWorker.done`은
Job·프로세스·이미지 pin 정리가 모두 성공한 후에만 닫는다.

## 제한된 연결 runner

`loginagent.RunConnection`은 service-child bootstrap이 제공하는 보호된 grant
reader와 데스크톱 입력 port만 받는다. 보호된 grant의 전체 선택 지문과 활성
로그인 전 콘솔을 확인하고, 기존 프로필에서 읽은 단일 상대와 TLS 1.3 mTLS를
사용한다. handshake마다 상대 인증서·역할·일반 입력 ALPN·현재 grant·프로필을
검사하며 session ticket 재개는 허용하지 않는다. 양쪽 역할에 맞는 listener/dial
경로를 선택한다. 주소는 현재 private/loopback 연결 프로필의 검증을 재사용한다.

수동 해제·승인 삭제/미확인·로그인·세션 변경·신뢰 철회·인증서 변경/만료·
주소/배치/입력 허용 변경은 새 handshake와 lease를 차단한다. 연결 중에는 1초
정책 관측으로 세션을 취소하고 supervisor의 기존 입력 정리와 listener 종료를
기다린다. 정책 파일 읽기는 입력 이벤트마다 수행하지 않는다. 화면 크기 변경도
연결을 중단한다. pairing·clipboard·update·discovery·사용자 관리 API·trust 수정·
인증서 갱신을 등록하지 않는다. TLS 키를 진단이나 JSON으로 반환하지 않는다.

이 runner는 아직 서비스나 EXE entry point에 연결하지 않았다. 실행 프로세스의
출처 확인, Winlogon 전용 입력 port, 로그인/충돌 시 입력 정리 확인을 다음
bootstrap 작업에서 결합해야 하므로 현재 배포 동작과 준비 CLI 상태는 유지한다.

## 서비스와 자식의 인증·정리 handshake

로그인 전 실행에 전용 SYSTEM-only 파이프 `Krasis.Login.Worker.v1.<pid>`를 사용한다.
서비스는 suspended 자식을 준비하며 파이프의 최초 인스턴스를 생성하고 실제
kernel client PID가 소유한 자식과 같은지 확인한다. 자식은 어떠한 제어 프레임을
읽기 전에 고정 설치 이미지, SCM 설정·PID와 실제 pipe server를 인증한다.
서비스·자식 모두 활성 로그인 전 콘솔과 원래 정책을 activation 직전에 다시 확인한다.
세션·승인 선택 지문·32바이트 난수 nonce를 고정 길이 프레임으로 서로 확인한다.
패킷으로 키·인증 자료·경로·명령·입력 값은 전달하지 않는다.

정리 요청은 runtime 취소를 전달한다. 자식은 runtime의 입력·네트워크 정리가
반환되고 제어 reader까지 join한 뒤에만 clean receipt를 보낸다. 잘못된 stop,
파이프 소실, runtime 실패와 정리 실패는 failed 또는 recovery 오류로 남는다.
서비스도 clean receipt를 받는 것만으로 성공하지 않고 소유 Job·실제 프로세스·
이미지 pin을 회수한다. 종료 응답은 3초, native 회수 대기는 1초로 제한한다.
활성화 쓰기 이후 응답이 불명확하거나 이 정리가 실패하면 다음 owner를 차단한다.

`RunInstalledLoginWorker`와 `launchLoginConnection`은 아직 EXE entry point와
서비스 관측 루프에 연결하지 않았다. Winlogon 입력 port와 사용자 handoff를
준비한 뒤 결합한다. 현재 일반 입력·설치·실행 중인 앱은 그대로 유지한다.

## Winlogon 전용 데스크톱 처리

`inputdesktop.RunWinlogon`은 authenticated service-child가 별도로 고른 고정
Winlogon 대상만 연다. 기존 `Run`의 Default 이름 검증은 유지한다. 일반 프로세스가
권한을 주장하는 callback을 넘겨도 실제 SYSTEM·UIAccess·System 무결성·활성
물리 콘솔 토큰이 아니면 scope를 열지 않는다. thread impersonation·restricted·
AppContainer 토큰도 거절한다. 이 검사는 토큰·DACL·정책을 변경하지 않는다.

소유 핸들의 고정 이름과 실제 입력을 받는지(UOI_IO)를 검증하고 전용 OS thread에
연결한다. 다른 데스크톱이나 비활성 핸들로 대체하지 않는다. callback 종료 후
원래 borrowed desktop을 복원하고 소유한 핸들만 닫는다. 잘못된 thread의 입력,
scope 취소, desktop 비활성화와 복원 실패는 기존 scope 규칙대로 거절한다.
이 API 자체는 입력 권한을 주지 않으며 서비스 인증·peer lease 검증·정리 확인을
runtime과 결합해야 한다. 실제 Windows 로그인 화면 입력은 아직 시험하지 않았다.

## Winlogon 입력 직렬화와 복구

`inputworker.RunLoginInput`은 인증된 child가 `RunWinlogon`을 열고 제한된
connection callback에 입력 port를 제공한다. callback은 별도 goroutine에서
실행하며 모든 key/button·move·wheel·cursor 조회·배치·readback 요청은
unbuffered mailbox를 통해 같은 데스크톱 소유 thread에서 순서대로 실행한다.
각 요청의 실행 직전에 scope의 취소·thread·활성 desktop 검사를 수행한다.
전달 전 취소는 NOT_APPLIED이며 전달 후 취소는 실제 결과를 기다린다. 실행
중 panic으로 결과가 사라지면 OUTCOME_UNKNOWN이며 재시도 가능한 실패로 바꾸지 않는다.

별도의 pressed journal은 실제 성공 또는 적용 여부 미확인인 press만 보존한다.
알려진 NOT_APPLIED press는 journal에서 제거하며 소유하지 않은 release는
native 입력을 보내지 않는다. connection 취소 후 새 press·move·cursor 작업은
차단하되 소유 key/button release에는 독립 정리 문맥을 허용한다. callback
종료·panic·Goexit 때도 callback을 join하고 같은 Winlogon scope에서 최대 2초
release recovery를 수행한다. 비활성화된 desktop의 남은 입력은 ErrRecovery와
Remaining으로 남기며 Default로 우회하거나 clean handoff로 표시하지 않는다.
scope 닫기 실패와 callback 실패도 최종 결과에서 보존한다.

이 port는 아직 제품 runtime에 연결하지 않았다. source hook의 각 native
thread도 Winlogon에 묶고, 인증된 child runner와 service owner에 결합해야 한다.

## 남은 커밋 단위

1. 인증된 child bootstrap에 Winlogon 입력 port와 제한된 mTLS runner를 결합한다.
   로그인 전에는 페어링·클립보드·업데이트·일반 관리 API를 제공하지 않는다.
2. source hook의 모든 thread를 Winlogon scope에 묶는다. Default나 WTS
   사용자 토큰 검증은 전역적으로 완화하지 않는다.
3. 로그인 전 owner의 정리가 확인된 뒤 사용자 에이전트로 넘긴다. 양쪽 재부팅,
   로그인·로그아웃·잠금·사용자 전환·네트워크 지연과 PIN 입력을 실기기에서 확인한다.

현재 이 모듈은 서비스 entry point나 UI 옵션에 연결하지 않았다. CLI의 관리자
준비·해제 작업은 구현했지만 실제 설치 파일에는 적용하지 않았다. 자동 시작 설정,
입력 서비스, 인증 자료와 실행 중인 연결은 변경하지 않았다. generation도 유지한다.
로그인 전 연결·입력 지원은 아직 완료되지 않았으며 실제 부팅 시험도 하지 않았다.
