# ADR 0064: 특권 입력 worker의 승인된 채널과 독립 복구

- 상태: Accepted for worker core; native authentication, service and product integration pending
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

제품 활성화 전 다음 조건이 추가로 필요합니다.

1. service는 fixed-name local-only pipe를 독점 생성합니다. 원격 pipe client는 거부합니다.
2. client PID는 wire가 아닌 Windows pipe metadata에서 얻습니다. 프로세스 핸들을
   유지하며 현재 console session, console user, 제한·AppContainer 여부와 정확한
   승인 agent EXE digest를 확인합니다. SID는 authorization 비교에만 쓰고 기록하지 않습니다.
3. agent도 service PID·SYSTEM token·설치된 EXE digest를 확인합니다. pipe 이름 또는
   client가 보낸 claim만으로 service 권한을 신뢰하지 않습니다.
4. verified agent의 기존 mTLS·peer trust·lease·epoch·sequence 승인 뒤에만 채널을
   엽니다. worker는 새 lease마다 새 binding과 pressed state를 사용합니다.
5. service가 고정 설치 위치의 자기 EXE만 console worker로 실행합니다. console 변경,
   agent 종료, 업데이트와 service stop은 소유한 worker를 정리합니다. client가 실행할
   파일이나 token source를 선택하지 못합니다.

이 기반은 아직 resident agent, 보호창 guard, updater 또는 설치 명령에 연결되지
않았습니다. SYSTEM service 설치와 A→B 원격 PIN 입력 성공을 의미하지 않습니다.
API·DB·배포 형식과 agent generation은 제품 통합 시 함께 변경합니다.

## 검증 범위

in-memory stream과 fake input으로 EOF·취소·절대 만료·replay·wrong binding·불확실한
press·해제 실패·malformed frame·응답 결과 분류를 검증합니다. 실제 입력은 전송하지
않습니다. Native process authentication과 SYSTEM worker 실행은 별도로 검증해야 합니다.

## 참조

- [GetNamedPipeClientProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeclientprocessid)
- [Named Pipe Security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)
- [Windows Go service API](https://pkg.go.dev/golang.org/x/sys/windows/svc)
