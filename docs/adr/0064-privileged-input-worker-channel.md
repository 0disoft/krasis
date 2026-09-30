# ADR 0064: 특권 입력 worker의 승인된 채널과 독립 복구

- 상태: Accepted for worker core and process-authentication primitives; service and product integration pending
- 결정일: 2026-09-30
- 관련: ADR 0062, ADR 0063, ADR 0036

## 입력 worker

`internal/inputworker`는 이미 승인된 한 원격 lease의 입력과 해제를 소유합니다.
wire 요청은 고정 크기 48바이트이며 허용하는 동작은 press, release, move, wheel,
heartbeat, close뿐입니다. 임의 PID, 실행 경로, desktop, 명령, token 또는 privilege를
받지 않습니다. 요청마다 정확한 순서 번호와 암호학적 무작위 128비트 채널 binding을
검사합니다. binding은 새 lease마다 바뀌며 process authentication의 대용이 아닙니다.

worker는 `inputdesktop`의 고정 OS 스레드에서 입력과 복구를 수행합니다. 입력은 기존
`application.InjectPress`/`InjectRelease`를 통해 처리하며 성공이 불명확한 press도
복구 계획에 유지합니다. 확실히 미전송된 press만 제거합니다. 응답은 결과 분류와
수락 개수만 포함하며 native error, 입력 내용, SID, 사용자 이름을 반환하지 않습니다.

입력 연결 EOF, 취소, 순서·binding 불일치, protocol 오류, 입력 실패, lease 만료와
close는 worker를 종료시킵니다. worker는 별도의 취소되지 않은 2초 복구 문맥으로
눌린 키와 버튼을 해제합니다. 실패한 해제 개수는 supervisor가 확인할 수 있도록
남깁니다. 동일 입력을 다른 backend에서 재전송하거나 복구 성공을 추정하지 않습니다.

무응답 한도는 2초이며 heartbeat도 승인된 lease의 절대 만료를 연장하지 못합니다.
reader는 한 개이며 새 요청은 무제한 queue에 쌓지 않습니다. response write에도
deadline이 있어 응답을 읽지 않는 client가 입력 복구를 무한히 막지 못합니다.
취소 시 stream을 닫고 reader가 종료된 뒤 복구합니다.

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

## 검증 범위

in-memory stream과 fake input으로 EOF·취소·절대 만료·replay·wrong binding·불확실한
press·해제 실패·malformed frame·응답 결과 분류를 검증합니다. 실제 입력은 전송하지
않습니다. zero authority·다른 session·낮은 integrity·restricted/AppContainer 거부,
agent exit 후 같은 logon의 복구와 close의 1회 실행도 검사합니다. 실제 Windows에서는
자기 process token metadata와 test EXE digest pin을 입력 없이 검사합니다. 실제 service와
agent의 pipe 상호 인증 및 SYSTEM worker 실행은 별도로 검증해야 합니다.

## 참조

- [GetNamedPipeClientProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeclientprocessid)
- [Named Pipe Security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)
- [Windows Go service API](https://pkg.go.dev/golang.org/x/sys/windows/svc)
