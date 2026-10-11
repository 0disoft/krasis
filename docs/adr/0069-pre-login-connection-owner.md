# ADR 0069: 로그인 전 연결의 서비스 소유권

- 상태: Accepted; runtime integrated in g262, physical reboot acceptance pending
- 결정일: 2026-10-10
- 관련: ADR 0036, ADR 0038, ADR 0064, ADR 0066, ADR 0068

기한 있는 v1 승인과 인증서 pin 정책은 기존 승인에 유지한다. g263부터의
영구 v2 승인·역할 키 기반 인증서 갱신은 ADR 0070이 이 부분을 대체한다.

## 문제

기존 Windows 자동 시작은 HKCU Run의 사용자 로그인 자동 시작이었다. 부팅 시
자동 시작하는 `KrasisInput` 서비스도 보호 입력만 담당하며 peer 연결을 만들지
않았다. 로그인 뒤의 Chrome PIN 창 제어 성공과 재부팅 직후 LogonUI 입력은
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

g267은 로그인된 활성 세션의 `WTSActive`와 클라이언트가 연결된
`WTSConnected`를 물리 콘솔 후보로 취급한다. 두 경우 모두 실제 활성 콘솔,
로컬 프로토콜, 완전한 빈 사용자 값과 조회 전후 동일 세션을 확인해야
`BeforeLogin`이 된다. 사용자 값이 있으면 기존 사용자 세션으로 취급한다.
RDP·연결 중·연결 해제·조회 실패는 계속 시작하지 않는다.
참조: [Microsoft WTS_CONNECTSTATE_CLASS](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/ne-wtsapi32-wts_connectstate_class).

서비스는 고정 보호 경로 `LOGIN-RUNTIME.json`에 시작 시각·최근 콘솔/승인
판단·마지막 자식 시작 결과·최근 실패 단계와 확보한 Win32 숫자를 기록한다.
상태 전환 때만 저장하고 사용자 이름·SID·입력·화면·주소는 넣지 않는다.
기록 실패는 권한 검사를 바꾸지 않는다. 진단은 이 기록을 `loginRuntime`으로
전송하고 잘못된 값은 incomplete로 구분한다. 로그인 후에도 최근 시작 실패를
유지한다. 기록은 과거 결정이며 현재 연결이나 실제 입력 성공의 증거가 아니다.

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

설정 준비 CLI는 `configured`, `active:false`를 반환한다. 설정 저장 결과를
현재 연결 성공으로 표시하지 않는다. g261부터 서비스가 이 설정을 사용한다.
현재 사용자 UI에는 아직 로그인 전 연결 옵션을 노출하지 않는다.

## 로그인 전 실행 primitive

서비스 전용 `launchLoginConsole`은 고정 설치·SCM 실행 여부·현재 물리 콘솔·
보호된 승인 기록·원래 사용자 정책을 시작 전과 자식 resume 직전에 다시 읽는다.
일반 worker는 계속 `--input-worker` / `winsta0\default`를 사용한다. 로그인 전
worker는 별도 고정 모드 `--login-worker` / `winsta0\Winlogon`만 선택할 수 있고
IPC로 실행 경로·명령·데스크톱을 선택할 수 없다. child entry point와 서비스
루프에 연결했으며 명령 직접 실행만으로 genuine service 인증을 통과할 수 없다.
로그인 전 child는 전체 고정 설치를 읽어 두 이미지 pin을 독립 확인하고, 일반
입력 child의 부분 설치 로딩/agent-image 전달 경로와 구분한다.

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

`RunInstalledLoginConnection`이 genuine service bootstrap의 session·승인 지문·
실제 service proof로 Winlogon 입력 port와 source hook을 만들고 이 runner에
연결한다. 시작·source hook 생성·경계 활성화에서 원래 승인/콘솔을 다시 확인한다.

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

`launchLoginConnection`은 서비스 owner의 Start backend이며 EXE의 고정
`--login-worker` 진입점은 `RunInstalledLoginConnection`에 연결된다. runtime
종료와 parent 콘솔 관측이 경합해도 정확한 parent가 최대 1초 안에 Stop을
보내면 정리 receipt를 받을 수 있다. Stop 부재·잘못된 바인딩·파이프 소실은
실패이며, 자발적 runtime 종료만으로 clean receipt를 보내지 않는다.
activation 직후 빠른 사용자 로그인이 Winlogon 준비를 중단시킬 수 있다.
입력/네트워크 callback이 한 번도 시작되지 않았고 desktop cleanup 실패도
없었던 경우만 전용 no-input preparation 표시를 남긴다. 이 표시도 정확한
parent의 Stop이 있어야 clean retirement로 처리한다. 실행 중 권한 오류나
입력 결과 미확인에는 이 표시를 붙이지 않는다.

## 서비스 자동 시작과 사용자 입력 handoff

설치기의 기존 SERVICE_AUTO_START 설정을 유지하며 `runInstalledLoginService`를
서비스 실행 루프에 연결한다. 서비스는 genuine normal pipe의 첫 인스턴스를
먼저 소유하고 ready record를 저장한 뒤 login owner를 시작한다. 이 pipe는
일반 accept 루프에 그대로 넘겨 readiness 저장과 endpoint 소유 사이에 틈을
만들지 않는다. 기존 설치 이미지·실제 SCM 프로세스·child 인증 검사는 유지한다.

250ms 콘솔/승인 관측과 일반 입력 lease의 admission은 같은 gate에서 직렬화한다.
일반 agent 인증 이후 user session을 확인하고 pre-login worker의 Stop/Done·
입력/네트워크/Job 회수를 확인한 뒤 콘솔을 재조회해야 일반 lease를 허용한다.
관측/승인/start 실패는 로그인 전 실행만 중단하며, 정리 미확인은 일반 lease도
차단하고 서비스 failed receipt로 이어진다. 서비스 종료도 monitor를 join하고
owner 정리를 기다린다. 승인 없음은 로그인 전 실행을 허용하지 않는다.

child EXE 진입점은 service bootstrap·Winlogon port·제한된 mTLS runner에 연결한다.

## Winlogon 전용 데스크톱 처리

### 사용자 세션이 이미 준비된 잠금 화면

g269 현장 진단에서는 PIN을 입력하기 전에도 WTS가 `USER_SESSION`을
보고했고 일반 agent가 연결을 소유했다. 잠금 커튼에서 PIN 화면으로 바뀌면
`fault_source_pointer_snapshot_failed`와 `lifecycle_desktop_switch`가 기록됐다.
사용자 값이 있다는 사실만으로 활성 입력 데스크톱을 Default로 간주할 수 없다.

`RunInteractive`는 service/agent 인증을 받은 설치형 입력 전용 worker에만
연결하는 별도 primitive다. SYSTEM·UIAccess·System 무결성·활성 콘솔 세션을
검증하고 Windows가 선택한 정확한 Default 또는 Winlogon 핸들만 연다.
입력마다 기존 scope의 활성 여부를 확인하고, 비활성이면 원래 thread desktop을
복원하고 핸들을 닫은 다음 새 활성 데스크톱을 연다. 이 thread에는 window나
hook이 없다. 정리 실패는 worker를 차단하며 입력을 재실행하지 않는다.
입력 lease와 pressed journal은 한 worker가 계속 소유한다. 데스크톱 전환 뒤의
key-up도 새로운 입력이 아닌 같은 journal의 해제로 처리한다. portable Run과
login source hook의 고정 데스크톱 정책은 유지한다.

g270의 실제 연결은 설치된 agent/service의 독립 인증을 통과한 일반 입력
worker에만 이 primitive를 연결한다. portable 및 test Serve는 Default 전용이다.
일반 agent가 secure desktop에서 source cursor를 읽을 수 없으면, 설치 backend가
선택된 연결만 receive-only hook을 사용한다. 로컬 source hook·raw mouse·capture를
설치하거나 active라고 표시하지 않고, 기존 mTLS/inbound 승인과 service lease로
들어오는 원격 입력만 유지한다. 일반 source 활성화는 이 상태에서 거부한다.
Chrome의 Default desktop PIN 경로는 그대로 유지한다.

g270 잠금 시험에서는 `lifecycle_session_lock` 뒤 일반 agent가 종료됐고,
약 48초 뒤 재시작했다. 이 구간에는 입력 lease가 준비되지 않았다. g271은
실제 설치 backend가 선택된 활성 연결에 한해 SessionLock/DesktopSwitch를
프로세스 종료 대신 연결 runner 취소로 처리한다. source hook·입력 해제·worker·
transport·guard 정리가 모두 반환된 뒤 250ms를 기다리고 현재 데스크톱으로
새 runner를 준비한다. secure desktop이 유지돼도 receive-only 수신을 시작하며,
Default로 돌아오면 같은 전환 경로로 로컬 capture를 다시 준비한다.

중복 전환은 정리 중인 runner만 취소하며 별도 worker를 동시에 만들지 않는다.
정리 오류·시간 초과는 기존 FAULTED 차단을 유지하며 자동 재시작하지 않는다.
portable/미확인 backend, 수동 해제, 갱신 중에는 기존 프로세스 종료 정책을
유지한다. 로그오프·세션 해제·절전·종료도 계속 프로세스를 중단한다.
로그인 전 grant·서비스의 사용자 세션 admission 정책은 바꾸지 않는다.

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

`wininput.NewLoginSourceHook`은 별도의 고정 Winlogon 모드다. 시작 관측 thread와
low-level hook·raw mouse·wheel capture thread 각각을 창/hook 생성 전에
Winlogon scope에 묶는다. 키·마우스 callback, raw packet, wheel 전달과 capture
활성화는 현재 thread/desktop을 다시 확인한다. 다른 desktop이면 전달을 중단한다.
로그인 뒤 비활성 capture를 정리할 때는 소유한 window·touchpad 등록·정확한
thread attach만 회수하고 이전 foreground 창을 다시 활성화하지 않는다.
로그인 전에는 사용자 Shell fullscreen COM broker를 시작하지 않는다. native
정리·desktop 복원 뒤에도 추가 OS-thread lock을 유지해 capture goroutine 종료가
해당 OS thread를 폐기하게 한다. 이전 Win32 message queue가 다음 capture에
재사용되지 않으며 일반 source hook의 thread 폐기 정책과 동일하다.

g269부터 destination 경계 복귀 때 커서 숨김의 준비 좌표도 shield window의
native owner message queue에서 읽는다. 일반 peer worker는 Winlogon에 연결되지
않았으므로 그 thread의 GetCursorPos를 사용하지 않는다. 조회 대기는 20ms로
제한하고 요청 ID·local input generation·활성 desktop을 확인한다. 늦은 응답,
로컬 입력, 새 entry는 오래된 숨김을 취소한다. 전역 cursor·ShowCursor count와
foreground는 변경하지 않는다.

로그인 child의 종료 실패는 인증된 고정 control pipe에서 한정된 종류만 서비스로
전달한다. desktop·policy·input·capture·connection·cleanup·unknown은 모두 실패
receipt이며 clean retirement로 승격하지 않는다. 서비스가 Stop의 실제 결과를
받은 후 LOGIN-RUNTIME.json의 worker 단계와 reason을 남긴다. launch·pipe·bootstrap·
activation 시작 실패도 별도 reason을 보존한다. 숫자 native 오류를 확보하지 못한
경우 known=false를 유지하며 화면·입력·창 이름·사용자 정보는 기록하지 않는다.

입력 port와 hook은 인증된 child runner와 service owner에 결합했다.

## 정상 취소와 정리 실패의 구분

취소된 session이 LOCAL이며 forwarding·worker join·native owner 정리·pressed
ledger 비움이 모두 확인되고 deadline 초과나 ErrRecoveryUnverified가 없어야
session의 연결 종료 원인을 정상 취소로 처리한다. 이후 supervisor의 connection/
source close 오류, Winlogon journal 잔여 입력, desktop 복원 오류와 parent Job
회수 실패는 계속 보존한다. 단순 errors.Is(Canceled) 검사는 joined cleanup
실패를 숨길 수 있어 쓰지 않는다. 서비스 StopPending도 기존 파이프의 Stop
수신을 막지 않으며, 새 입력이나 새 권한을 부여하는 동작은 허용하지 않는다.

## 배포와 실기기 수용 기준

g262에 runtime과 고정 child 진입점을 포함한다. UI/사용자 agent는 로그인 후
기존 자동 시작 경로를 사용한다. 로그인 전에 GUI·관리 API를 열지 않는다.
사용자 설정의 자동 시작 허용과 관리자 승인 기록이 모두 필요하다. 양쪽에서
기존 연결을 정상 확인하고 최신 보호 입력 설치를 적용한 뒤, 각 PC의 현재
상대 별칭으로 `krasis-install --prepare-login PEER`를 관리자 실행해 승인한다.
해제는 `--disable-login`이며 설정의 기존 만료·철회 규칙을 유지한다.

양쪽 재부팅, 한쪽만 재부팅, 실제 LogonUI 입력, 로그인 후 일반 agent handoff,
로그아웃·잠금·사용자 전환·네트워크 지연과 grant 만료/철회를 실기기에서
확인해야 한다. 현재 코드/모의 실패 경로 검증은 완료했지만 실제 부팅 입력
시험은 아직 하지 않았다. 실제 설치 서비스·승인·인증 자료·실행 중 연결은
이 구현 작업에서 변경하지 않는다.
