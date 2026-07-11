# ADR 0004: Phase 0 입력 crash guard를 별도 프로세스로 둔다

- 상태: Accepted
- 책임자: cherr
- 범위: `krasis-lab agent-session` Phase 0 실험

## 맥락

destination agent가 합성 key-down 또는 button-down을 보낸 뒤 죽으면 process 내부
`PressedState`와 cleanup 코드도 함께 사라진다. peer가 살아 있어도 죽은 destination의
Windows 입력 상태를 대신 해제할 수 없다. down 주입 뒤 파일 journal을 갱신하는 방식은
두 작업 사이의 crash에서 token을 잃고, 파일을 먼저 쓰는 방식은 실제 down 전 crash에
불필요한 up을 만들 수 있다.

## 결정

- Phase 0의 `agent-session`은 같은 `krasis-lab.exe`를 숨겨진
  `crash-guard-internal` 역할로 별도 실행한다.
- agent와 guard는 상속된 익명 stdin/stdout pipe만 사용한다. listener port, named pipe,
  shell, current directory 탐색 또는 장기 저장 journal을 만들지 않는다.
- agent는 discrete down 전에 `PREPARE_PRESS`를 보내고 guard의 ACK를 받은 뒤에만
  `SendInput`을 호출한다. up 주입이 성공한 뒤 `RELEASED`를 보내고 ACK를 받는다.
- parent가 죽어 pipe가 EOF가 되거나 protocol 오류, write 실패 또는 정상 disarm이
  발생하면 guard는 기억한 token을 최초 down의 역순으로 모두 up 주입한다.
- protocol은 version이 있는 12-byte 고정 command와 8-byte ACK만 허용하고 최대 256개
  pressed token을 유지한다. 원시 문자, pointer 좌표, session ID, 인증서 또는 window
  정보를 전달하거나 기록하지 않는다.
- 각 IPC round trip과 guard 종료 대기는 2초로 제한한다. ACK가 오지 않으면 pipe를 닫고
  guard를 종료해 agent session을 실패시키며 runtime lock을 무기한 점유하지 않는다.
- 정상 종료도 guard가 남은 token을 해제한 뒤에만 성공한다. guard가 먼저 종료되면
  agent session을 실패시키며 재연결을 계속하지 않는다.
- 이 결정은 설치 topology를 확정하지 않는다. 정식 agent, single-instance, code signing,
  process ACL과 updater 경계는 Phase 0 뒤 별도 ADR에서 결정한다.

## 결과

destination agent 단독 crash 뒤에도 독립 process가 합성 key/button을 해제할 수 있고,
subprocess test로 parent 강제 종료와 EOF 복구를 재현할 수 있다. down 직전의 guard ACK는
stuck input보다 드문 불필요한 up을 택하는 fail-safe 순서다.

guard 자체와 agent가 동시에 죽거나 OS가 중단되는 경우, Windows 보안 데스크톱,
관리자 권한 경계, 실제 물리 key와 같은 token이 동시에 눌린 경우의 영향은 이 구조만으로
증명되지 않는다. 실제 네 대상 장비의 강제 종료 실험을 통과하기 전에는 제품 안전성으로
승격하지 않는다.

## 대안

- process 내부 cleanup만 사용: destination process death를 복구할 owner가 없어 기각한다.
- agent가 직접 durable file journal을 갱신: write와 `SendInput` 사이의 순서 구멍과
  Windows ACL·내구성 부담 때문에 Phase 0 기본안으로 채택하지 않는다.
- kernel driver: 현재 user-session 범위를 크게 넘으므로 Phase 0 결과가 필요성을 입증할
  때만 재검토한다.
