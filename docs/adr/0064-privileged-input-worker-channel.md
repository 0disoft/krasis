# ADR 0064: 특권 입력 worker의 승인된 채널과 독립 복구

- 상태: Accepted for worker, service bootstrap and SCM entry point; installation/update and product integration pending
- 결정일: 2026-09-30
- 관련: ADR 0062, ADR 0063, ADR 0036

## 입력 worker

`internal/inputworker`는 이미 승인된 한 원격 lease의 입력과 해제를 소유합니다.
wire 요청은 고정 크기 48바이트이며 허용하는 동작은 press, release, move, wheel,
heartbeat, close 및 아래 커서 I/O 확장입니다. 임의 PID, 실행 경로, desktop, 명령, token 또는 privilege를
받지 않습니다. 요청마다 정확한 순서 번호와 암호학적 무작위 128비트 채널 binding을
검사합니다. binding은 새 lease마다 바뀌며 process authentication의 대용이 아닙니다.

worker는 `inputdesktop`의 고정 OS 스레드에서 입력과 복구를 수행합니다. 입력은 기존
`application.InjectPress`/`InjectRelease`를 통해 처리하며 성공이 불명확한 press도
복구 계획에 유지합니다. 확실히 미전송된 press만 제거합니다. 응답은 결과 분류와
수락 개수와 커서 조회에 필요한 고정 좌표만 포함하며 native error, 키 입력 내용, SID, 사용자 이름을 반환하지 않습니다.

입력 연결 EOF, 취소, 순서·binding 불일치, protocol 오류, 입력 실패, lease 만료와
close는 worker를 종료시킵니다. worker는 별도의 취소되지 않은 2초 복구 문맥으로
눌린 키와 버튼을 해제합니다. 실패한 해제 개수는 supervisor가 확인할 수 있도록
남깁니다. 동일 입력을 다른 backend에서 재전송하거나 복구 성공을 추정하지 않습니다.

무응답 한도는 2초이며 heartbeat도 승인된 lease의 절대 만료를 연장하지 못합니다.
reader는 한 개이며 새 요청은 무제한 queue에 쌓지 않습니다. response write에도
deadline이 있어 응답을 읽지 않는 client가 입력 복구를 무한히 막지 못합니다.
취소 시 stream을 닫고 reader가 종료된 뒤 복구합니다.

## 실패 단계 관측

실패 응답의 v6/v7은 scope 진입, 입력 적용, 커서 readback, executor 완료 중
실제로 실패한 단계 하나를 고정 값으로 전달합니다. 기존 apply/submission suffix를
유지하고 헤더 bit 30으로 legacy bucket을 구분합니다. bucket을 측정된 millisecond로
바꾸지 않습니다. 실패 suffix의 예약 바이트만 단계 값으로
사용합니다. 성공 응답의 바이트와 실패의 certainty·수락 개수는 바꾸지 않습니다.
입력 이동 후 readback이 실패하면 여전히 outcome-unknown이며 재전송하지 않습니다.
이전 실패 응답도 읽으며 단계 값이 없으면 미확인으로 남깁니다.

source 교체가 거부되면 현재 owner가 입력 전달 중인지, idle gate가 거부했는지,
등록된 owner가 없었는지도 구분합니다. idle gate 거부를 특정 키나 버튼 때문이라고
단정하지 않습니다. 이 고정 코드에는 입력값·커서 좌표·자격 증명 내용이 없습니다.

최종 복구 보고서가 없으면 서비스는 job 정리 전에 정확히 소유한 자식 process
handle을 읽어 `workerExit`를 기록합니다. 상태는 RUNNING/EXITED/UNAVAILABLE이며
종료 코드·확인 여부는 원래 Win32 오류와 별개입니다. 종료 코드 0도 복구 성공
증거가 아니므로 보고서가 없을 때의 ErrRecovery는 그대로 유지합니다. PID 재조회,
추가 대기·서비스 재시작·입력 재전송은 하지 않습니다. 입력 ACK의 시간 초과·EOF·
닫힌 채널·순서 불일치도 고정 코드로 남기며 다른 오류는 미분류로 남깁니다.

## Windows 전달 경계

프로세스 인증 모듈은 아래 2·3의 기반을 구현합니다. 제품 활성화 전 전체 경계를
연결해야 합니다.

1. service는 fixed-name local-only pipe를 독점 생성합니다. 원격 pipe client는 거부합니다.
2. client PID는 wire가 아닌 Windows pipe metadata에서 얻습니다. 프로세스 핸들을
   유지하며 현재 console session, console user, 제한·AppContainer 여부와 정확한
   승인 agent EXE digest를 확인합니다. agent는 Program Files의 고정 보호 위치
   `Krasis/Input/krasis-agent.exe`에서 실행되어야 합니다. 사용자 쓰기 가능한 경로에서
   이름과 해시만 비교하면 loaded image와 교체된 pathname이 달라질 수 있으므로
   portable agent는 특권 endpoint에 직접 연결할 수 없습니다. SID는 authorization
   비교에만 쓰고 기록하지 않습니다.
3. agent도 service PID·SYSTEM token·설치된 EXE digest를 확인합니다. pipe 이름 또는
   client가 보낸 claim만으로 service 권한을 신뢰하지 않습니다. 조회에는 SCM connect와
   service query 권한만 요청하며 관리자 권한을 요구하는 ALL_ACCESS를 사용하지 않습니다.
4. verified agent의 기존 mTLS·peer trust·lease·epoch·sequence 승인 뒤에만 채널을
   엽니다. worker는 새 lease마다 새 binding과 pressed state를 사용합니다.
5. service가 고정 설치 위치의 자기 EXE만 console worker로 실행합니다. console 변경,
   agent 종료, 업데이트와 service stop은 소유한 worker를 정리합니다. client가 실행할
   파일이나 token source를 선택하지 못합니다.

이 기반은 아직 resident agent, 보호창 guard, updater 또는 설치 명령에 연결되지
않았습니다. SYSTEM service 설치와 A→B 원격 PIN 입력 성공을 의미하지 않습니다.
API·DB·배포 형식과 agent generation은 제품 통합 시 함께 변경합니다.

`Authority`의 zero value는 항상 거부됩니다. 인증된 agent의 kernel process handle과
image 읽기 핸들을 수명 동안 유지하고, 요청 전 process 생존·현재 console session과
logon authentication ID를 다시 검사합니다. 종료 시 핸들을 한 번만 해제합니다.
agent 종료 후에도 같은 console logon의 key-up 복구는 허용하지만, 다른 logon으로
바뀌면 이전 입력의 해제를 주입하지 않습니다. native 오류에 owner·경로 정보가
있더라도 공통 오류로 치환합니다.

특권 service 설치와 자동 업데이트는 이 고정 보호 위치의 image·승인 digest를
transaction으로 함께 갱신해야 합니다. 일반 사용자가 쓴 digest나 updater의 파일
경로만으로 service EXE를 교체하는 endpoint는 허용하지 않습니다. 설치 및 배포 신뢰
경계가 완성되기 전에는 현 portable agent를 승격하거나 보호창 guard를 끄지 않습니다.

## Native 파이프 전송

`AcceptAgent`와 `ConnectService`는 고정 이름 `Krasis.Input.v1`만 사용합니다. 서버는
SYSTEM·session 0에서만 열며 first-instance·single-instance·remote-client rejection을
설정합니다. interactive 사용자는 stream·attribute 접근만 가지며 추가 server instance를
만드는 FILE_CREATE_PIPE_INSTANCE 비트는 포함하지 않습니다. 수신 전에 agent 인증을,
client가 server 응답을 신뢰하기 전에 service 인증을 수행합니다. `Channel`의 stream과
authority는 외부에서 바꿀 수 없는 한 쌍입니다. service proof를 agent 입력 승인으로
사용할 수 없습니다.

overlapped I/O는 버퍼를 pin하고, 시간 초과·취소·close 때 정확한 요청을 취소한 뒤
kernel completion을 기다립니다. 그 전에 버퍼를 unpin하거나 이벤트·pipe handle을
닫지 않습니다. 동시 close는 한 번만 handle을 닫고 동일한 완료를 기다립니다.
파이프 연결은 각각 최대 5초·2초이며 송수신 deadline도 pending I/O에 적용됩니다.
service·agent 상호 인증과 console worker 실행은 source에서 연결했으며 실제 설치 상태의
SYSTEM 경로와 원격 입력은 아직 검증하지 않았습니다.

## 콘솔 worker 실행 기반

`LaunchConsoleWorker`는 SYSTEM·session 0·고정 설치 위치의 service와 인증된 agent
authority에서만 실행됩니다. service의 승인 digest로 자기 EXE를 읽기 고정하고,
현재 console session의 System32/winlogon.exe 토큰만 복제합니다. 복제한 토큰에
UIAccess를 설정하고 생성된 자식의 실제 SYSTEM·UIAccess·session·제한 여부를
재검사한 뒤 실행합니다. 토큰 준비 전후 agent/logon/console 승인을 재확인합니다.

필요한 privilege는 전용 OS 스레드의 복제 impersonation token에만 적용합니다.
service process와 다른 스레드의 토큰은 변경하지 않으며 impersonation 해제 실패 시
그 스레드를 runtime pool로 반환하지 않습니다. 원시 오류·SID·사용자명은 반환하지
않습니다. 환경은 빈 Unicode block, desktop은 winsta0/default, 인수는 고정 worker
모드이며 client가 경로·PID·token source·session·명령을 고르지 못합니다.

자식은 생성 순간부터 kill-on-close Job에 속하며 활성 프로세스 한도는 1입니다.
생성 후 job 할당까지의 service crash 공백이 없습니다. 인증 취소·세션 변경·agent
종료 후에는 bounded recovery 기회를 두고 소유 job을 닫습니다. service stop/업데이트
supervisor는 먼저 입력 채널을 닫고 해제 결과를 기다린 뒤 worker를 종료해야 합니다.
강제 process 종료 자체는 눌린 키·버튼 해제의 성공 증거가 아닙니다.

service entry point·worker bootstrap은 아래 초기화 경로로 연결했습니다. 설치·제품
adapter는 아직 연결하지 않았습니다. 서비스 또는 worker를 실제 설치·실행하지 않았고 agent
generation은 유지합니다. 일반 권한 launch 거부, native Job 한도, 동시 close를
입력 없이 검사하며 SYSTEM child 실행과 원격 PIN 입력은 별도 기기 검증이 필요합니다.

## 에이전트 입력 client

`Client`는 인증된 service proof만 받아 기존 `application.InputPort`를 구현합니다.
agent proof를 service proof로 사용하지 못합니다. service bootstrap에서 동의한
lease/binding/절대 만료를 사용하며 입력·heartbeat는 하나씩 송수신합니다. 응답의
순서·동작·결과 분류·예약 바이트와 수락 개수를 검사하고 native 오류는 반환하지
않습니다. lease 만료·service 종료·잘못된 응답·전송 실패 후 같은 채널을 재사용하지
않습니다. 입력을 보낸 뒤 응답이 유실되면 `OUTCOME_UNKNOWN`으로 반환하며 같은
입력을 재전송하거나 일반 backend로 옮기지 않습니다.

호출 취소는 pending read/write를 중단합니다. 채널 close가 성공한 사실과 worker
해제 복구 성공은 구분합니다. 해제 결과는 service supervisor가 별도로 받아야 합니다.
현재 client도 제품 agent에 연결하지 않았으며 bootstrap·설치 신뢰 경계가 완성되기
전에는 보호창 guard를 대체하지 않습니다. fake input의 실제 client/worker 왕복,
EOF 복구·취소·응답 유실·잘못된 응답·미전송/부분전송 결과 분류를 검사합니다.

## Service/worker 초기화와 해제 영수증

`ConnectLease`는 이미 승인된 원격 lease를 인증된 service에 전달하며 desktop worker의
준비 응답을 기다립니다. lease frame은 고정 432바이트이고 세 식별자·binding·절대 만료만
포함합니다. 빈 값·비정규 padding·예약 바이트·만료·1시간을 넘는 만료를 거부합니다.
frame은 새로운 인증이나 mTLS 승인을 대신하지 않습니다.

service는 자식이 suspended인 동안 SYSTEM-only private pipe를 만들고, 이미 인증한
agent pipe의 핸들만 해당 자식에 복제합니다. client가 핸들이나 PID를 제공하지 않습니다.
worker는 실제 service PID·image digest를 재검사하고, 복제 pipe의 kernel client PID로
agent를 다시 인증합니다. Win32 [DuplicateHandle](https://learn.microsoft.com/en-us/windows/win32/api/handleapi/nf-handleapi-duplicatehandle)
계약에 따라 복제 핸들의 소유권은 자식에 있으며 부모는 승인·image pin을 별도로 유지합니다.
worker가 console logon을 다시 확인하도록 복제 token의 SeTcbPrivilege를 명시적으로
활성화합니다. service process token은 변경하지 않습니다.

worker는 실제 desktop scope 준비 후에만 ready를 알립니다. 종료 control은 정리 요청
하나뿐이며 명령·경로·token 변경을 받지 않습니다. worker는 독립 입력 해제가 끝난 뒤
수락 동작 개수와 해제 attempted/released/remaining만 반환합니다. supervisor는 이
영수증을 기다린 후 Job을 정리합니다. 누락·잘못된 영수증과 남은 입력은 recovery unknown
또는 failure이며 다음 lease를 승인하지 않습니다. process exit 자체를 해제 성공으로
간주하지 않습니다. 이 초기화 경로는 SCM entry point에 연결했으며 설치·업데이트와
resident agent 입력 adapter 활성화는 아직 연결하지 않았습니다.

## SCM 실행과 보호된 설치 기록

`cmd/krasis-input`는 `--service`와 `--input-worker` 두 고정 모드만 허용합니다.
service는 SCM에서만, worker는 실제 console의 SYSTEM/UIAccess token에서만
실행합니다. client가 다른 PID·파일·세션·명령을 고르는 실행 모드는 없습니다.

`LoadInstallation`은 `%ProgramFiles%/Krasis/Input/INPUT-INSTALL.json`의
`krasis.input.install/v1` 기록과 고정 agent/service EXE를 읽기 고정합니다. JSON에는
schemaVersion·agentSHA256·serviceSHA256만 허용합니다. Krasis/Input 디렉터리,
기록과 두 EXE에서 reparse point를 거부하고 owner가 SYSTEM/Builtin Administrators이며
다른 주체의 쓰기·삭제·DACL 변경 권한이 없는지 확인합니다. 일반 사용자 소유 디렉터리,
파일명만 맞춘 portable agent, user-writable digest와 요청 경로는 승인 근거가 아닙니다.
설치 코드가 이 owner·DACL·정확한 두 image digest를 하나의 transaction으로 구성해야 합니다.

SCM stop/shutdown은 새 승인을 중단하고 현재 worker의 해제 결과를 기다립니다.
그동안 StopPending/checkpoint를 유지합니다. 남은 입력·없는 영수증·Job 정리 실패는
service failure로 반환하며 정상 종료나 다음 lease로 숨기지 않습니다. service·worker
설치 또는 권한 변경을 실행하지 않았으며 새 binary는 빌드 검사만 수행합니다.

## 설치 transaction의 복구 순서

`install_transaction.go`는 설치·업데이트·제거의 내부 상태 전이를 구현합니다. Windows
고정 위치의 파일 staging·journal·복원과 SCM 제어 backend를 구현했습니다. 제품 updater와
배포 installer에는 아직 연결하지 않았으며, 외부에서 호출할 수 있는 설치 명령도 없습니다.
target digest는 향후 독립적으로 승인된 설치 payload에서만 구성해야 합니다.
원격 묶음의 checksum/BUILD.json 또는 agent 요청은 실행 승인 근거가 아닙니다.

한 번에 한 installer만 고정 설치 위치를 소유합니다. 기존의 검증된 두 EXE와 실행
상태를 기록하고, candidate 및 rollback copy를 먼저 준비한 다음 worker의 clean
release를 확인합니다. 그 후 고정 파일·SCM 구성을 교체하고 시작·trust·readiness를
확인해 commit합니다. zero target은 기존 설치 제거이며 같은 rollback을 사용합니다.

각 효과 전에 durable journal의 단계를 저장합니다. journal에는 단계·두 image digest·
설치/실행 여부만 포함하며 경로·명령·PID·입력 내용은 포함하지 않습니다. 정규 형식·
필수 필드·크기를 검사하며 duplicate/unknown field와 불가능한 상태는 거부합니다.
journal은 복구 위치를 설명할 뿐 설치 권한을 부여하지 않습니다. Windows backend는
record의 protected owner/DACL, fixed paths 및 pinned image를 별도로 검증해야 합니다.

commit 전 중단은 rollback 방향으로만 복구합니다. 교체 또는 새 서비스의 준비 실패 시
별도의 20초 문맥에서 candidate를 clean stop하고 이전 두 image·SCM 구성을 복원합니다.
이전 서비스가 실행 중이었을 때만 재시작하며, 첫 설치 실패는 설치 전의 부재로 돌아갑니다.
키·버튼 해제 영수증을 검증하지 못하면 복원·다른 backend 시작·artifact 삭제를 하지 않고
journal을 유지합니다. 사용자 취소는 복구 문맥을 취소하지 않습니다. journal write 결과가
불확실하면 추가 효과를 중단하고 다음 실행에서 실제 durable record를 대조합니다.

durable commit 이후 artifact 정리 실패는 이미 확인한 새 설치를 되돌리지 않습니다.
다음 실행에서 새 설치를 재검증하고 정리를 완료합니다. 정리가 끝나기 전 새 transaction은
시작하지 않습니다. backend의 단계별 효과는 crash 이후 idempotent해야 하고 Finish는
transaction 소유물만 제거한 뒤 journal을 마지막으로 삭제해야 합니다.

가짜 backend로 첫 설치·업데이트·제거, 각 단계의 crash, journal 저장 전/후 오류,
candidate 준비 실패, 입력 해제 실패, 취소, commit 후 정리 실패와 lock 충돌을 검사합니다.
이 검사는 실제 Program Files 교체 또는 서비스 설치 성공을 의미하지 않습니다.

## Windows 설치 파일의 소유권과 교체

`installFiles`는 이미 상승한 installer에서만 OS의 Program Files 위치를 선택합니다.
일반 agent를 상승시키거나 환경 변수·RPC 경로를 받지 않습니다. 기존에 소유권이나
DACL이 안전하지 않은 Krasis 디렉터리가 있으면 ACL을 덮어쓰지 않고 거부합니다.
생성 객체는 Administrators 소유이며 SYSTEM·Administrators에만 쓰기 권한을,
Users에는 읽기·실행만 부여합니다. root의 읽기 핸들을 유지해 다른 위치로 바뀌지 않게
하고, 공유를 허용하지 않는 고정 lock 파일로 installer 간 실행을 직렬화합니다.

candidate·rollback·retired 디렉터리는 각각 `Input.stage`·`Input.rollback`·
`Input.retired`로 고정합니다. journal에는 실행 승인으로 쓰지 않는 digest와 단계만
저장합니다. pending 파일은 기존 owner/DACL·reparse·hardlink를 확인한 뒤 쓰고 flush하며,
동일 root 안에서 MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)로 교체합니다.
이는 프로세스 중단 복구 절차이며 모든 저장장치의 전원 장애 내구성을 증명하지 않습니다.

새 파일은 trusted installer의 독립 승인된 embedded payload에서만 받도록 내부 생성자를
분리했습니다. 두 SHA-256 외에 PE amd64·Go command identity·Windows/amd64·CGO off·
clean Git revision·두 image의 동일 revision을 확인하고 바이트를 복제해 보관합니다.
이 검증은 publisher 서명 대신이 아니며, 아직 배포 installer에 embedded payload를
연결하지 않았습니다. 원격 bundle checksum이 맞는 것만으로 호출할 수 없습니다.

기존 image를 읽기 고정해 rollback copy를 검증한 뒤 candidate를 준비합니다. 교체는
active를 retired로 옮기고 candidate를 active로 옮기는 두 단계이며, 그 사이의 중단은
journal에 따라 rollback합니다. 기존 agent가 파일을 잡아 첫 rename이 실패했으면
원본을 삭제하지 않고 그대로 보존합니다. 정리는 재귀 삭제 없이 세 고정 파일만 대상으로
하며, 예상하지 못한 파일·하위 디렉터리·링크·권한이면 중단하고 journal을 유지합니다.
Windows 파일 검사는 사용자 소유 파일의 수정 거부·내용 보존·취소·zero store를
검사하며 실제 Program Files 설치는 실행하지 않습니다.

## 서비스의 준비·종료 영수증

SCM의 Running/Stopped 상태만으로 endpoint 준비 또는 입력 해제를 추정하지 않습니다.
service는 고정 보호 root의 `INPUT-SERVICE.record`에 80바이트 정규 record를 저장합니다.
내용은 phase·kernel PID·run별 무작위 식별자·설치된 service digest뿐이며 입력 payload나
사용자 식별자는 포함하지 않습니다. 기록을 읽는 installer도 보호 owner/DACL·고정 image·
SCM과 실제 process metadata를 별도로 검증해야 합니다.

service는 starting 기록을 먼저 쓰며 first-instance pipe 생성 후 ready를 원자적으로
저장한 다음에만 agent 인증과 lease 수신을 진행합니다. ready 기록 실패 시 채널을 닫고
입력을 받지 않습니다. ready 전 실패는 입력이 없었다는 clean 기록을 남길 수 있습니다.
ready 이후 worker 복구 실패는 failed이며, 정상 복구가 완료된 경우에만 clean을 씁니다.
clean 저장 실패도 SCM failure로 반환해 installer가 프로세스 종료만으로 교체하지 않게
합니다. 기록은 service 취소와 독립된 2초 문맥에서 flush·교체합니다.

입력 없는 fake run으로 ready 전 승인 차단·중복 ready·복구 실패·취소 이후 기록·
종료 기록 저장 실패 및 wire 형식을 검사합니다. 실제 SCM 설치와 원격 제어 검증은
아직 수행하지 않았습니다.

## 검증 범위

`windowsInstallBackend`가 transaction을 고정 Windows 파일 작업과 SCM 제어에 연결합니다.
이미 상승한 installer와 보호 root에서만 생성하며 backend 생성자와 payload는 private입니다.
자동 업데이트·CLI·UI에는 실행 endpoint를 노출하지 않습니다. SCM manager에는
connect/create만 요청하고, 제어 전에 고정 own-process·demand-start·LocalSystem·quoted
EXE와 --service 인수·빈 dependency/group·owner/DACL을 확인합니다. Users에는 query/read만
허용합니다. crash restart·custom command·trigger도 없어야 하며, 다른 기존 서비스는
덮어쓰거나 삭제하지 않습니다. 생성 직후 권한 설정 실패 시 아직 실행하지 않은 그
서비스만 제거를 시도하고 transaction record를 보존합니다.

Start는 실제 PID의 session 0·SYSTEM·System integrity·비제한 token·고정 보호 digest 및
그 PID와 digest의 ready record까지 확인합니다. Stop은 process handle을 유지하고 clean
record와 실제 종료를 기다립니다. 이미 멈춘 서비스에도 clean record가 필요하며 SCM의
never-started 상태만 입력이 없었던 별도 경우로 취급합니다. 해제 실패·missing/failed
record·timeout은 교체·복원을 진행하지 않습니다. 두 rename 사이의 중단은 retired/rollback
image로 종료를 대조한 후 복원합니다. 제거는 SCM object 부재까지 기다리고 lifecycle
record를 삭제한 후 journal을 마지막으로 지웁니다. 보호 root와 lock anchor는 유지합니다.
현재 demand-start 계약이며 부팅 정책과 제품 agent 실행 전환은 후속 통합 대상입니다.

config·ACL 거부, kernel 문자열 pointer 범위, zero backend, clean build payload의 command
role·바이트 소유권을 입력 없이 검사합니다. 실제 Program Files 배치·SCM 등록/시작/종료와
A→B 입력은 실행하지 않았으므로 native API 조합의 실기기 결과는 아직 미검증입니다.

in-memory stream과 fake input으로 EOF·취소·절대 만료·replay·wrong binding·불확실한
press·해제 실패·malformed frame·응답 결과 분류를 검증합니다. 실제 입력은 전송하지
않습니다. zero authority·다른 session·낮은 integrity·restricted/AppContainer 거부,
agent exit 후 같은 logon의 복구와 close의 1회 실행도 검사합니다. 실제 Windows에서는
자기 process token metadata와 test EXE digest pin을 입력 없이 검사합니다. 실제 service와
agent의 pipe 상호 인증 및 SYSTEM worker 실행은 별도로 검증해야 합니다.
실제 Windows 파이프에서 bytes·kernel PID metadata, pending read의 timeout 후 재사용,
pending read 중 동시 close와 first-instance 중복 거부도 입력 없이 검사합니다.

2026-10-01 집중 검사에서 inputworker/inputdesktop 테스트, checkptr 검사,
go vet와 `cmd/krasis-input`의 Windows EXE 빌드가 통과했습니다. SCM 상태 변경은
가짜 worker로 검사했으며 실제 SYSTEM 서비스·다른 세션으로의 핸들 복제·원격
마우스/키보드/PIN 제어는 이 검사에 포함되지 않습니다. 설치·업데이트와 제품
활성화 전이 끝나기 전에는 agent generation을 104로 유지합니다.

## 내장 payload 설치기

`cmd/krasis-install`은 `--install` 또는 `--remove` 하나만 받습니다. 외부 파일 경로,
manifest, checksum, 임의 서비스명은 받지 않습니다. Go로 구현하며 별도 C++ 런타임을
추가하지 않습니다. 일반 Go 빌드는 payload가 없어 설치를 거부하고, 전용 builder만
clean committed revision의 agent·input EXE와 두 SHA-256을 `go:embed`로 내장합니다.
실행 시 해시뿐 아니라 PE·Go command role·revision·수정되지 않은 source 조건도 검증합니다.

전용 설치 EXE에는 Krasis 아이콘과 `requireAdministrator`, `uiAccess=false` 매니페스트가
들어갑니다. installer 자체에 UIAccess를 부여하지 않으며 제품 agent를 자동 상승시키지
않습니다. 이미 상승한 실행만 기존 고정 보호 root와 SCM transaction에 연결됩니다.
적용 문맥은 60초이고 rollback은 기존 취소 독립 문맥을 사용합니다. 제거도 같은 복구
경로를 거치며 보호 root·lock anchor를 남깁니다. 인증서 저장소나 Windows 보안 정책은
변경하지 않습니다.

결과는 `krasis.input.install-result/v1` JSON으로 code·state를 반환합니다. 종료 코드
0은 installed/removed, 2는 invalid_arguments, 3은 installation_denied, 4는
previous_installation_restored, 5는 recovery_required, 6은 결과 출력 실패입니다.
네이티브 오류 문자열·경로·사용자 식별자·입력 내용을 출력하지 않습니다. 결과를 못 썼다는
코드 6은 설치가 수행되지 않았다는 뜻이 아니므로 재실행 전에 상태 대조가 필요합니다.

g110은 기존 code·state에 선택적인 `diagnostic`과 `diagnosticSaved`를 추가합니다.
진단에는 최초 실패의 고정 stage·transaction phase·operation, 숫자 Win32 오류와
`win32Known`, 복원 결과 및 복원 실패의 별도 stage·operation·숫자 오류가 들어갑니다.
핸들 정리 실패도 `closeOperation`·`closeWin32Error`로 분리하므로 최초 오류나 확인된
복원 결과를 덮어쓰지 않습니다. 숫자를 확인하지 못하면 `win32Known:false`, 오류 0으로
표시하며 이것은 Win32 성공을 뜻하지 않습니다. 오류 문자열과 파일 경로는 저장하지 않습니다.

검증된 receipt는 receiving user의 보호된 diagnostics 폴더에
`installation-latest.json`으로 원자적으로 저장됩니다. 저장 실패는 결과 JSON의
`diagnosticSaved:false`로 표시하며 실제 설치 결과를 바꾸거나 자동 재시도하지 않습니다.
일반 `diagnostics export`와 peer 진단 전달에도 이 receipt를 포함합니다. 수신 측은
고정 코드·필드·숫자를 다시 검증하고, 잘못된 receipt는 제거한 뒤
`installationIncomplete:true`로 표시합니다. g108·g109의 과거 실패에는 원래 숫자
오류가 남아 있지 않으므로 새 빌드의 B 실기기 receipt가 원인 확정에 필요합니다.

g110 B 실기기에서 최초 journal 저장이 `save/stage/file_move`, Win32 `32`로
실패했습니다. 로컬 Windows 재현에서도 작성 핸들을 닫은 상태에서 상위 폴더의
`FILE_SHARE_READ` pin이 journal 이동을 막았고, directory pin에 `FILE_SHARE_WRITE`를
함께 지정하면 신규 저장·기존 journal 교체·자식 파일 정리가 성공했습니다.
g111은 directory handle에만 이 공유 모드를 적용합니다. `FILE_SHARE_DELETE`는
허용하지 않아 폴더 자체의 교체를 막으며, 개별 metadata·EXE handle의 write/delete
차단과 소유자·DACL·reparse·단일 링크 검증은 유지합니다. 같은 pin을 쓰는 서비스
receipt 저장과 설치 정리 경로에도 적용됩니다. journal 교체 전에 닫는 검증 핸들의
close 실패도 숫자 진단으로 반환하며 이동을 진행하지 않습니다.

`journal_unknown`은 Save 오류 이후 durable journal 상태를 확정하지 못해 추가 설치·
복원·정리를 수행하지 않았다는 뜻입니다. 복원 오류 번호 미확인은 복원 성공이 아니며,
다음 attended 설치기의 기존 journal 검증/복구 경로가 상태를 대조합니다. 이 실패를
자동 재시도하거나 transaction 파일을 수동 삭제하지 않습니다. native 이동 재현과
복원 경계 테스트의 성공은 B 제품 서비스 설치 또는 A→B PIN 입력 성공을 뜻하지 않습니다.

g111 B 실행은 서비스·파일 생성 후 종료 코드 2와 stdout 부재로 보고됐으며, stderr와
최종 native 단계는 미확인입니다. 코드 2의 정상 인자 오류 경로도 JSON을 쓰므로 이
관측을 인자 오류 또는 특정 panic으로 확정하지 않습니다. 서비스 1077은 시작하지 않은
상태의 관측이며 설치 완료나 복원 성공을 증명하지 않습니다.

g112 설치기는 native backend 동작 직전에 같은 보호 diagnostics 위치에 미완료
checkpoint를 저장합니다. `inProgress:true`, `result:5`, `recovery:required`와 고정
`activeStage`·`activePhase`는 마지막으로 시작하려던 작업이며 성공/실패 결과가 아닙니다.
최초 실패와 복원 실패는 기존 필드에 유지합니다. 정상 반환 후 최종 receipt가 checkpoint를
대체하며, 중간 종료 시 이전 세대 결과만 남는 것을 줄입니다. checkpoint 저장은 best-effort,
설치·복원 순서는 불변이고 panic·오류 내용은 기록하지 않습니다. 실패한 저장이나 fatal
종료까지 보장하지 않으며, 최종 JSON·stderr·서비스·transaction 상태를 함께 대조해야 합니다.

g111 부분 설치 후 파일 hash만 맞는 설치 에이전트를 선택하면 stopped 서비스에
원격 제어 lease를 요청해 A→B 전환 실패/재연결을 반복할 수 있습니다. g113은 UI의
설치 에이전트 선택 전에 fixed SCM 설정·서비스 보안 정보·Running/PID와 보호된
service-ready receipt의 phase/PID/image digest/run을 읽기 전용으로 대조합니다.
직접 실행된 설치 에이전트의 privileged backend 선택에도 같은 준비 검사를 적용합니다.
준비되지 않은 서비스는 시작·수정하지 않고 기존 검증된 portable 에이전트 경로를
선택합니다. 일반 제어 복구와 관리자/PIN 입력은 별도이며, 실제 privileged lease의
인증·권한·회수 검증은 설치된 에이전트가 계속 수행합니다. 실행 후 서비스가 종료되는
경합은 기존 입력 회수와 연결 실패 처리에 맡기며 제어 중 자동 backend 전환은 하지 않습니다.

g114는 SCM 출력 버퍼를 `uint64` backing storage로 만들어 native 구조체 정렬을
보장합니다. Go 스택의 byte 배열은 8-byte 정렬을 보장하지 않습니다. 로컬 read-only
EventLog 조회에서 8-byte 정렬은 성공했고 4-byte 어긋난 주소는 QueryServiceConfig
내부 접근 위반 `0xc0000005`와 종료 코드 2를 재현했습니다. 기본 설정·failure actions·
trigger 조회 모두 같은 bounded 정렬 버퍼를 사용합니다. 서비스 설정·권한·복원 순서는
변경하지 않습니다. 이 수정의 로컬 조회 성공과 B의 실제 설치·PIN 입력 성공은 구분합니다.

g115는 설치 성공 후 remote lease 연결 실패를 고정 `installed_input_stage_*` 코드로
구분합니다. pipe open, SCM/서버 PID·설정·상태·프로세스·이미지 검증, lease metadata/
write, worker-ready 응답과 client 검증 중 최초 실패를 보존합니다. 알려진 native 오류는
2/5/32/231/1314와 other/unknown으로만 분류하며 원문·사용자·경로·입력은 기록하지 않습니다.
기존 인증·회수·backend 선택 규칙은 불변입니다. 설치 성공은 실제 lease-ready 성공을
보장하지 않으며 이 진단만으로 실패 원인을 확정하거나 PIN 제어 성공을 주장하지 않습니다.

`scripts/build-input-installer.mjs`는 source clean 확인 후 revision별 출력 폴더를 만들며
기존 산출물은 덮어쓰지 않습니다. 실행 파일의 실제 PE resource에서 아이콘과 관리자
매니페스트를 대조합니다. 관리자 resource는 일반 테스트 EXE에 영향을 주지 않도록
빌드 후 제거합니다. builder는 installer를 실행하지 않습니다. 현재 설치기는 서명 없는
로컬 산출물이고 자동 업데이트 전달·실제 서비스 설치·제품 입력 활성화는 후속 작업입니다.
이 단계에서도 배포 generation은 104로 유지합니다.

## 설치기 전달과 이전 버전 호환

g105부터 current UI·agent·CLI 묶음에 `krasis-install.exe`가 선택적으로 포함될 수
있습니다. installer가 있으면 checksum·PE·Go command role·동일 revision·clean source와
`inputinstaller` build tag도 검증합니다. 기존 legacy UI 묶음이나 g104 이하 metadata에는
허용하지 않습니다. Pack→Read→Stage는 같은 파일 목록을 사용하며 UI 실행 대상은 계속
`krasis.exe`입니다. installer 수신은 설치·상승·실행 승인이 아닙니다.

g104 이하의 기존 수신기는 새 파일을 거부하므로 g105 배포 폴더에는 설치기를 넣지
않습니다. g106 이상을 보내는 UI는 상대가 g90–g104이면 먼저 고정 g105 폴더를 선택하고,
그 폴더가 없으면 TRANSITION_REQUIRED로 중단합니다. 더 오래된 기기는 기존 g75/g90
전환을 우선합니다. g105 탐색은 제한된 BUILD.json을 읽는 routing hint이고 실제 전송은
기존 trust·version·bundle 재검증을 거칩니다. 자동 재전송은 추가하지 않습니다.

g106 이상 builder는 현재 revision의 설치기를 만들거나 같은 revision 산출물만 사용합니다.
내장 agent가 일반 업데이트 agent와 동일하도록 GUI linker flags를 맞추고 SHA-256을
대조합니다. 설치기를 checksum 목록에 포함한 뒤 실제 bundle Pack·Read·Stage와 기존 UI
launch 준비를 검사합니다. 설치기는 실행하지 않습니다. 실제 서비스 설치와 원격 PIN
제어는 제품 입력 경로 연결 및 실기기 검증이 남아 있습니다.

### g116: ordinary-agent service proof without OpenProcess

g115 실기기에서 설치는 성공했지만 일반 agent의 `OpenProcess`가 최소
`PROCESS_QUERY_LIMITED_INFORMATION`에도 오류 5를 반환했습니다. 서비스 설정과
파이프 조회는 통과했고 `service_process_open`에서만 실패했습니다.

g116의 서비스 인증은 보호된 SCM object의 fixed own-process·LocalSystem·command·
security/failure/trigger 검증, 고정 서비스 image의 digest/write lock/ACL 검증을
그대로 유지합니다. `GetNamedPipeServerProcessId`가 반환한 실제 kernel pipe owner와
`QueryServiceStatusEx`의 Running own-process PID가 같아야 합니다. service handle은
Authority가 소유하고 채널 종료 시 닫으며, 이미지 pin 이후와 매 사용 전 같은 kernel
PID/SCM 상태를 재확인합니다. stopped/pending/shared-process/PID 변경/조회 실패는
즉시 거부합니다. SCM의 지연된 상태나 PID 재사용이 있어도 기존 pipe의 I/O 실패는
fatal이며 입력 재전송·backend 전환을 하지 않습니다.

일반 agent는 서비스 process/token을 열지 않으며 ACL·권한·UAC 설정을 바꾸지 않습니다.
서비스의 자기 SYSTEM/session 확인과 agent의 console logon·image 인증은 유지합니다.
SCM+pipe owner는 서비스 인증 근거이고, 단독 PID·파이프 이름·receipt는 권한이 아닙니다.
회귀 검증은 실제 native pipe만으로 서비스 권한을 만들지 못하는 경우와 stop/restart·
다른 PID·상태·service type·조회 오류를 검사합니다. A→B/PIN 효과는 실기기에서 별도로
확인해야 합니다.

### g117: privileged input latency stages

g116 실기기에서 원격 PIN Esc·취소 클릭과 양방향 입력에 성공했지만 버벅임이
보고됐습니다. g117은 권한·데스크톱 검증을 줄이지 않고 비용부터 분리합니다.
ordinary client의 mutex 대기·SCM/pipe authority 확인·request write·response 대기,
worker의 console authority 확인·desktop scope 부대 비용·실제 apply를 측정합니다.
worker scope는 전체 executor 시간에서 apply 시간을 뺀 값으로 scheduling 비용도
포함하며, response 대기는 worker 비용·pipe 전달·scheduling을 포함하므로 서로
더해서 전체 지연으로 해석하지 않습니다. Heartbeat도 같은 측정 창에 포함합니다.

응답 길이는 24 bytes로 유지하고 v2의 기존 reserved 3 bytes에 worker의 coarse
duration bucket만 넣습니다. 값은 0(미측정), 1–5(<1ms/<10ms/<100ms/<1s/≥1s)만
허용하며 네 번째 reserved byte는 0입니다. 기존 v1은 reserved bytes가 전부 0인
경우만 계속 허용하고, 빈 v2·범위 밖 bucket·잘못된 sequence/op/status/certainty는
거부합니다. worker timing은 유효한 matching acknowledgement 이후에만 집계합니다.
서비스·agent image digest 일치 검증은 기존대로여서 버전이 다른 설치 파일을 섞어
실행하지 않습니다. 인증·입력 순서·expiry·release recovery·unknown 결과의 재전송
금지는 변경하지 않습니다.

로그는 15초 창마다 stage별 최대 bucket을 고정 `installed_input_timing_*` 코드로
최대 7건 기록하고 lease 종료/실패 때 남은 창을 한 번 flush합니다. 로그 sink panic은
입력 성공/실패를 변경하지 않습니다. 입력 종류·키·좌표·PIN·사용자·경로·오류 원문은
추가하지 않습니다. g117은 진단 보강이며 latency 개선을 주장하지 않습니다.

### g118: idle heartbeat yields to input

g117 진단에서 client lock과 response의 100ms–1s 최대 bucket이 관측됐지만
heartbeat와 입력이 섞여 전체 체감 지연 원인을 확정할 수 없었습니다. 재현 테스트는
매 250ms maintenance가 진행 중인 입력 mutex 뒤에 대기하고, 성공 응답 직후에도
다시 동기 왕복하는 중복 비용을 확인했습니다.

g118 heartbeat는 TryLock 실패 시 대기하지 않습니다. 진행 중인 입력이 bounded I/O와
worker recovery를 소유합니다. 잠금을 얻으면 기존 failure·context·lease expiry와
sequence 상한을 먼저 확인하고, 최근 250ms 안에 matching success acknowledgement가
있으면 추가 request를 보내지 않습니다. 실제 입력 성공도 worker idle deadline을
갱신하므로 활동 중 별도 maintenance가 필요하지 않습니다. heartbeat nil은 건강
확인 receipt가 아니며, 다음 idle tick이 채널 확인을 담당합니다. 성공하지 않은 응답은
생존 근거가 아니고, idle timeout 2초·lease 최대 시간·입력 순서·매 실제 입력의
client/worker authority 확인·lost ack의 unknown/no-retry 정책은 유지합니다.

실제로 전송한 heartbeat는 별도 `installed_input_heartbeat_timing_*` 최대 bucket으로
집계합니다. 기존 `installed_input_timing_*`는 g118부터 실제 입력만 포함합니다.
각각 15초 창당 최대 7개 고정 코드이며, 생략된 heartbeat는 측정값을 만들지 않습니다.
응답 프로토콜과 worker는 바꾸지 않습니다. 측정값을 합산하지 않으며, 실기기 전환
지연 개선 및 PIN 입력 유지 여부는 새 설치 agent로 별도 확인해야 합니다.

## 참조

- [GetNamedPipeClientProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeclientprocessid)
- [GetNamedPipeServerProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeserverprocessid)
- [QueryServiceStatusEx](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-queryservicestatusex)
- [Named Pipe Security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)
- [Windows Go service API](https://pkg.go.dev/golang.org/x/sys/windows/svc)
- [Windows service security objects](https://learn.microsoft.com/en-us/windows/win32/secauthz/securable-objects)
- [Service trigger configuration](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_trigger_info)

## g173 cursor I/O extension

Cursor read and absolute cursor set share the existing authenticated lease,
sequence, binding, desktop scope, deadlines and failure recovery. No session,
process or desktop selector is accepted. Cursor set is refused while the worker
owns pressed keys/buttons. The native backend must validate the target against
its current display bounds. Missing cursor capability fails without fallback.

Requests remain 48 bytes. A successful cursor-read or move-with-position ACK
appends exactly eight bytes (two signed 32-bit coordinates) to the existing
24-byte response, emitted in one pipe write; other
ACKs retain their existing size. A truncated suffix, wrong operation/sequence or
lost ACK closes the channel without replay. Coordinates stay in the private local
channel and are never added to product logs or support exports. Agent and worker
must be updated together before enabling this capability in the product adapter.

Move-with-position applies relative motion and snapshots the resulting cursor
within one authorized desktop scope and one request/ACK roundtrip. A snapshot
failure after successful motion is outcome-unknown, never not-applied. The
ordinary agent uses the matching ACK position for return-boundary detection
without a second cursor request. Entry read/set/verification and rollback retain
their transaction semantics, with all cursor I/O delegated to the selected worker.

## g180 confined cursor placement

Protected entry adds `CursorPlacementPort` and local operation `opCursorPlace`.
The worker observes `GetClipCursor`, intersects its rectangle with current screen
bounds, computes a permitted target, sets the cursor once and reads it back within
one existing authorized desktop scope. It never changes confinement. The expected
target comes from the observed rectangle, independently of actual readback.

The 48-byte request still contains only the requested coordinates, sequence and
lease binding. A successful placement ACK has a private sixteen-byte suffix:
actual X/Y followed by expected target X/Y. The client validates the matching ACK,
and the agent compares actual position with expected target before committing
entry. Ordinary cursor read/move ACKs and their eight-byte suffix are unchanged;
truncated placement suffixes fail closed. Agent and installed worker must both
support this operation; no compatibility retry or ordinary-backend fallback is
allowed after a missing or failed ACK.

Placement is refused while the worker owns pressed keys/buttons. Cancellation or
invalid confinement before mutation is not-applied; lost ACK or failed cursor
readback after a successful write is outcome-unknown. The adapter retains pending
rollback ownership and rejects real readback mismatches. The prior-cursor snapshot,
commit, rollback comparison and protected lease retirement remain unchanged.
Coordinates and confinement rectangles do not enter diagnostics. Native PIN
reentry and clipping-related disconnect recovery require device validation.

Windows can adjust a requested position to keep it inside confinement; see
[SetCursorPos](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setcursorpos)
and [GetClipCursor](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getclipcursor).

## g181 bounded cursor mismatch observations

`opCursorPlaceProbe` performs the same placement under the same lease, sequence,
pressed-state guard and desktop scope. Its successful ACK appends eighteen private
bytes: the existing actual/target coordinates, then `known` and `flags` bytes.
This adds no request, scope, write or roundtrip; the suffix shares the existing
response read. Ordinary placement keeps its sixteen-byte suffix. Agent and
installed worker must be updated together; an unsupported or lost ACK is never
retried using another operation or backend.

The client requests this variant once per tracked installed receiver entry, within
the existing 120-second/12-attempt collector window and a 64-request collector
cap across reconnect. Native port copies share a separate atomic 64-probe cap
for their worker lifetime. Untracked, source, expired or cancelled entries do not
request it. Zero metadata denotes an absent observation; malformed metadata is
consumed and dropped without changing the placement receipt.

Bits 0..5 mean target adjusted, readback matches target, confinement changed,
virtual-screen bounds changed, physical cursor matches target, and physical cursor
matches readback. `known` distinguishes an observed false comparison from an
unavailable query. The first two comparisons come from existing local values.
Only a mismatch queries confinement again, virtual-screen bounds again and
`GetPhysicalCursorPos`, once each in that order with cancellation checks. A failed
query remains unknown; it does not change input outcome or trigger another write.
Physical comparisons are recorded as a pair from one physical snapshot.

Only an `entry/mismatch` injection-failure record may export this comparison-only
observation. Logging copies it into the same failure record, and both local and
peer sanitizers validate its presence mask and flags. Positions, rectangles,
desktop/window identities and error text remain private. Sequential snapshots
cannot establish causality: physical movement, concurrent changes and coordinate
space differences still need device evidence. Matching placement and subsequent
ordinary pointer input add no diagnostic native queries. Entry rejection,
rollback ownership, uncertainty and protected lease retirement remain unchanged.

Physical-coordinate API contract:
[GetPhysicalCursorPos](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getphysicalcursorpos).

## g208: Cancellation while waiting for input ownership

Client request serialization and the resident protected-lease gate now use a
zero-value cancellation-aware mutex. No polling loop or detached lock waiter is
created. The client starts its existing two-second budget before lock acquisition,
so lock, authority and pipe I/O share one deadline. A canceled waiter sends no
request, consumes no sequence and is classified NOT_APPLIED. It does not close
the channel owned by an earlier in-flight operation. Heartbeat still uses
nonblocking TryLock. Protected admission and preparation honor their caller's
deadline without changing lease identity, approval, retirement or image pins.

## g283: Preserve the failed capture operation

The authenticated 32-byte worker report writes version 2, retaining a closed
source-operation code and the original nonzero Win32 number when available.
Readers also accept version 1 with its original zero padding. Success reports
cannot contain failure metadata; unknown codes, invalid padding and uncertain
release reports remain rejected. These fields are diagnostic, not cleanup proof.
The service runtime's lastFailure.operation preserves the operation through
endpoint joins without exposing input, window names, paths or error text.

## g284: Final worker reports have their own write deadline

Capture readiness previously left its five-second control write deadline in
place. A source owner outliving that deadline could not deliver its final clean
retirement or detailed failure report, causing the supervisor to require recovery.
Capture, cursor-presentation and input workers now share final-report delivery:
a fresh one-second write deadline is set only after native owners and release
work have joined. It does not inherit caller cancellation or retry input. Deadline
setup or report delivery failure still requires recovery and retains the original
worker cause. Actual cleanup failures and missing receipts remain fenced.
The regression uses an already expired net.Pipe write deadline without sleeps,
Windows input, or service mutation; clean and failed reports must both arrive.

## g285: Preserve failures through the service recovery fence

Capture and regular input endpoints retain the authenticated worker failure and
concurrent channel-close failure when recovery is required. A remaining pressed
journal no longer erases the worker operation or original native error either.
The service still stops on uncertainty; no failed cleanup becomes retryable.

Missing/truncated/invalid final reports are classified as `worker-report-read`;
expiration of the existing final-recovery deadline is `worker-report-timeout`.
These service-local diagnostic labels distinguish absent recovery evidence from
a received native failure. They do not change the worker wire format, extend
deadlines, expose raw error text, or grant release proof. g284's reported B service
failure remains unresolved until its original operation or missing-report cause
can be observed; this diagnostic correction alone is not a runtime fix claim.

## g286: Bound native Raw Input busy reads before routing

B's g285 service recorded capture / raw-read / Win32 170 at
2026-10-13T11:03:52.6759082Z. The raw-read label included both GetRawInputData
and visitor desktop validation, so this receipt does not identify the exact API.
Failures now retain raw-size, raw-packet, or raw-desktop through the authenticated
worker report and service receipt. New numeric operation codes are appended;
existing codes retain their values.

Only GetRawInputData returning UINT(-1) with ERROR_BUSY may repeat the current
undispatched message query. It is limited to three calls per query, a ten-ms
elapsed budget and one-ms cancellation-aware waits. The in/out capacity is reset
before each call. No packet is routed until a complete, validated read succeeds;
the visitor is invoked once. Visitor, desktop validation, access denial, invalid
handles, malformed packets, buffered reads, input actions and cleanup are never
retried. Persistent busy reads still stop with the original stage/native cause.
Cancellation ends read-only work and native owners must still join as before.

The native wrapper retains the Go byte buffer across the syscall. Tests inject
native query outcomes without operating Windows input: busy on either query then
success delivers once, permanent/canceled failures never deliver, and a visitor's
ERROR_BUSY is not replayed. B's real service persistence and login transitions
remain an attended verification requirement.

## Capture native cleanup diagnostics after g286

B's g286 failure at 2026-10-13T11:44:43.2124663Z was capture / raw-mouse /
Win32 170, following desktop-switch/source-refresh. It was outside the detailed
raw-size/raw-packet/raw-desktop labels and is not evidence of a data-read failure.
Raw-window destruction and early raw-device unregister failures are now retained
in the native result. Fixed desktop bind/restore/close/thread-exit operation
labels survive source-component wrapping and authenticated worker reports.
Existing numeric codes retain their values; appended labels contain no desktop
name, handle, path, input content or raw native message. Cleanup failures remain
fatal and cannot become a successful desktop transition.

## g287: Dispose capture threads before desktop handle release

Installed source capture binds the same fixed, privileged Default/Winlogon
desktop on a disposable locked OS thread. A retained, non-inheritable kernel
thread handle is duplicated before desktop acquisition. After callback and native
owner cleanup, the goroutine returns without unlocking the thread. Its exact
kernel handle must signal termination within one second before the owned HDESK
is closed by the waiting parent. SetThreadDesktop is not used to restore these
terminated capture threads. Borrowed original handles are never closed.

Unconfirmed thread exit retains the possibly attached desktop until process
retirement and reports desktop-thread-exit/ErrCleanup. Native window/hook release,
CloseDesktop and thread-handle close errors remain fatal; thread exit cannot erase
callback, panic, Goexit or cleanup failures. Ordinary injection and presentation
retain their restoration policy and token/installation/lease checks are unchanged.
Native desktop current/open/name/activity/bind/restore/close operations have fixed
diagnostic labels, so raw-mouse cannot hide the particular failing desktop call.

Deterministic tests gate handle close on exact thread exit, retain all cleanup
causes, and cover canceled, panicking and Goexit callbacks. A Windows test pins
and joins a real disposable thread without creating windows, registering input,
injecting input or changing desktop. Ordinary processes remain rejected from
protected capture. B's actual service-maintenance result remains unverified.

The raw-read busy tests now inject a clock and wait outcome: scheduler oversleep
must retain the original busy failure and never extend the ten-ms elapsed budget.
Production retry limits and delays are unchanged.

## g288: Separate agent retirement notification from native cleanup proof

B's g287 service survived ordinary use for over six minutes, then failed during
lock at 2026-10-14T07:14:19.3335601Z with capture and no operation/native number.
The precise g287 branch remains unconfirmed. A reproducible source-worker path
promoted a failed final sourceStop reply to ErrRecovery even after native capture
and sender retirement succeeded. A disappearing agent pipe could thus stop the
service despite its separate authenticated control channel remaining usable.

Finalization now joins both capture and sender results before deciding recovery.
Only a fully clean known transition may avoid ErrRecovery. A failed final agent
notification is reported as source-retirement-write on the separate service
control pipe, without inventing native cleanup uncertainty. The agent still
fails its unacknowledged operation and may reconnect; no input is replayed and
no new lease is authorized by this notification. The service's child report,
owned-process retirement and channel-close requirements remain independent.
Missing service reports, thread/join/native cleanup failure and close uncertainty
remain fatal. Authorization and desktop gates are unchanged.

Fixed source-capture-join/source-sender-join/source-command-loop/retirement-write/
channel-close labels are appended to existing report codes. The actual report
writer now accepts those fixed worker operation wrappers, retaining a deeper
native source operation when present. No raw error text, keys or paths are saved.
A real paired-pipe test disconnects the agent, exercises both final packet modes,
and passes the result through the independent report reader/service decision.
Failed cleanup and sender joins still fence that same path. B's attended lock,
PIN and login transitions remain required to establish whether g288 fixes the
reported failure.

## g289: Actual lease entry preempts optional preparation

One matched g288 installed handoff retained 114982us of preparation overlap in
its lease-admission mutex wait. Actual entry now publishes priority and cancels
the current optional preparation before acquiring that same mutex. Preparation
rechecks priority after publishing its cancellation owner, closing the race
between the first check and entry. New hints cannot queue ahead of pending entry
or shutdown. The mutex continues to join preparation and fence installation-pin
disposal; cancellation never substitutes for cleanup completion.

The authenticated service watches only the otherwise-unused preparation channel
while constructing an unbound console. Peer EOF or unexpected extra data cancels
that construction. Its native builder and the watcher are joined before another
request is accepted. A completed candidate that lost cancellation is closed;
close uncertainty remains ErrRecovery. The normal successful path interrupts and
joins its watcher before writing the unchanged preparation receipt. No peer byte
can choose a process, desktop or command, and no input/lease authority is added.

Deterministic tests first reproduce entry waiting on optional work, then verify
entry-triggered cancellation cannot bypass gated cleanup. Paired-pipe tests cover
peer disconnect, extra bytes, success receipt and preserved recovery errors.
The existing warm-child expiry, claim, trust, bootstrap and service fences stay
unchanged. B's actual latency and lock/PIN continuity require an attended test;
g288's separate native SendInput delay is not addressed by this scheduling change.

## g292: Keep capture off Go's non-terminating startup thread

B's g291 capture failure reported desktop-thread-exit without a native number.
The existing real Windows retirement test reproduced the same one-second failure
locally. Go runtime.mexit parks its initial OS thread (m0) instead of terminating
it. Returning from a locked capture goroutine on m0 therefore cannot signal its
retained kernel thread handle, even when capture cleanup has completed.

Package initialization records the startup thread identity. The capture launcher
locks its candidate thread before selecting it. If it is the startup thread, a
guard retains that lock until a replacement goroutine has locked another native
thread; the guard acquires no desktop, window, hook or input state, then unlocks.
The replacement retains its lock through exit and the existing exact-handle
termination proof still gates HDESK release. No timeout or cleanup fence is
removed. WAIT_TIMEOUT now preserves ERROR_TIMEOUT instead of an unnumbered error.

The original native-thread regression failed before this change and passed 100
times afterwards. Tests assert that no capture scope uses the startup thread;
they create no Windows input or capture windows. Actual A/B transitions remain
an attended check after both failed services have been safely replaced.

## g292: Distinguish retired capture from a failed forwarding command

A's paired g291 receipt at 2026-10-14T10:39:56.1116272Z reported capture /
source-command-loop. The finalizer previously added ErrRecovery to every command
or forwarding failure, even after native capture and the sender had both joined.
A source-only forwarding pipe owns no remote injection journal. Its failed
command still fails, but is not evidence that the native owners remain active.

Only the source writer marks its own forwarding/authority failures. The finalizer
requires independently clean native retirement and a joined sender with only
marked forwarding failures or known transition causes. It retains command-loop
and sender failures in the authenticated service report without promoting them
to native recovery uncertainty. Unknown sender failures, cleanup/join timeouts,
ErrRecovery, native cleanup errors, missing reports, process retirement and channel
close failures keep their existing service fence. No input is replayed.

Final sourceStop/sourceRetired packets communicate native retirement proof rather
than successful execution of a preceding command. The original command rejection
and the separate failed service report remain intact. Such proof authorizes no
input or desktop; the agent may refresh its source hook only under its existing
peer/session/installation gates.

Tests run the real source sender against a lost-authority/disconnected pipe,
verify suppression is inactive after join, then pass its result through the
worker report reader and service decision. They also verify that native/sender
cleanup failures and an independent channel close failure still stop the service.

## g293: Keep authenticated negative boundary queries nonfatal

A's paired incident shows boundary_admission_failed and network disconnection at
2026-10-14T10:39:56.082Z before the service's source-command-loop failure. The
source adapter erased the distinction between an explicit authenticated negative
query response and lost authentication/transport. The ordinary boundary gate
already keeps ErrBoundaryInspectionUnavailable nonfatal and grants no movement.

Only the matching, validated sourceAdmit negative reply is mapped to that existing
temporary query-unavailable result. Empty/lost replies, invalid sequences, wrong
operations, transport/authorization failures before a response and recovery errors
remain fatal. A fresh motion may request a fresh inspection; no query rejection
is promoted to allowed and no activation or input is replayed. Native desktop
cleanup failure during a source command explicitly retires the worker with
ErrRecovery; rejection handling cannot leave an uncertain native owner serving.

Tests cover negative and then successful queries on the same source owner using
the actual packet codec and request gate, with zero activation epoch throughout.
They retain the existing invalid-reply retirement and peer orchestrator recovery
tests. The historical g291 query failure lacks native detail, so its exact native
cause remains unknown; these fixes address the observed failure propagation.

## Source cleanup proof after g293 review

The full Windows/Go 1.26 repository reproduced two review failures: an internal
source sender remained blocked after lease expiry yet retirement/report status
was successful; a joined cancellation erased an independent cleanup error.
SourceSession now marks every independent cleanup timeout with
ErrSourceCleanupIncomplete, including interrupt, sender and failure callback
join. That marker survives lease expiry and source transport wrappers and keeps
retirement status failed and service report recovery-required.

Only an error tree consisting entirely of expected cancellation is normalized.
Generation changes no longer erase arbitrary errors: control's explicit unsent
source-action marker is shared by peer and protected source writers, and only
that pure marker may be discarded after the generation changes. No transport
failure or joined sibling inherits the marker's safe-disposal meaning.

Tests use actual SourceSession and final report writer/reader. They hold a sender
past expiry, interrupt/failure callback past the independent cleanup deadline,
and combine cancellation, discard and cleanup failures. Fixtures release and
join their intentionally blocked work. Clean expiry and genuinely unsent stale
actions remain normal; no timeout budget, journal or service fence is weakened.

## Preserve source reader, command and negative reply outcomes

The command reader now joins a terminal error channel before the native final
report. Validation/authority failures and external EOF, truncation or timeout
are retained independently of worker cancellation. Its private cancellation
context interrupts only the exact overlapped read and leaves the duplex writer
available for final retirement. A native-pipe test verifies cancellation, final
writing and subsequent reading without replacing or closing the channel.

Only the producer's actual GUI-inspection marker after a returned desktop scope
can generate a retryable sourceAdmit rejection. Authority, protocol, cancellation,
inactive desktop, callback and cleanup failures cannot inherit that meaning.
Cleanup uncertainty retires the worker before a query rejection is written.
The first safely rejected command is retained separately from continuation;
reply delivery failures join rather than replace the executed operation error.

Normal receive, EOF and deadline/cancellation races share one reply consumer and
one matcher. A buffered negative reply remains a rejection, and a buffered wrong
reply remains a protocol failure. Neither may be consumed and discarded before
a second read. Tests cover all three races and the actual Windows sourceClient
with an ordered write barrier, preserving zero activation epoch for queries.

Reader diagnostics carry only closed operation codes: EOF, truncated, timeout,
closed, invalid, authority or other read failure. They survive the authenticated
final service report. Missing input reports likewise distinguish EOF, truncation
and invalid data without changing the recovery fence; the exact owned process
exit observation still occurs before job cleanup. Unknown numbers remain unknown,
and no private exception text or input content is serialized. Numeric child
report operation codes are appended so existing code values do not change.

## g297: Coalesce desktop refresh during protected capture startup

The g296 paired collection `6261840b98e20a1cbee3939bdac89380` retained
service READY on both machines but recorded B's desktop switch, full session
reconnect, then a source refresh failure during the new owner's startup. The
service start times did not change. The first full reconnect's refresh-refusal
reason was absent because those fixed event codes were missing from productlog's
allowlist; no specific refusal reason is inferred from that absence.

A verified idle refresh now fences readiness immediately but coalesces events
while a protected owner is still starting. The existing bounded startup finishes
before the readiness callback requests native retirement, so cancellation does
not sever the startup IPC before its sourceStop proof can be read. Only the
replacement may resume capture. Ordinary shutdown still cancels immediately;
startup failure, failed retirement and joined cleanup errors remain failures.

Tests hold startup behind a barrier, deliver duplicate desktop-switch events,
then verify exactly one replacement, one readiness notification and no readiness
from the old owner. A joined cleanup failure prevents replacement. The existing
active-owner and physical-press fences remain unchanged. Fixed refresh refusal
codes (active_owner, capture_busy, unavailable) now survive sanitized export;
input contents and private error text are not added. Hosted tests do not establish
whether the first observed lock reconnect is resolved on real machines.
