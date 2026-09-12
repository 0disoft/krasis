# 제품 페어링 사용자 흐름

- 상태: Active contract
- 결정: `docs/adr/0035-product-identity-and-mutual-pairing.md`

## 새 기기 연결

1. 두 장비의 resident agent가 각자 `%LOCALAPPDATA%\Krasis\identity`에서 device identity를 연다.
   처음 실행이면 protected product root와 generation 1 identity를 만든다.
2. 시작 장비가 5분 유효 pairing offer를 만들고 상대 장비가 response를 보낸다. payload에는 public
   identity와 현재 transport fingerprint·역할만 있으며 private key와 certificate body는 없다.
3. 두 화면이 같은 6자리 code와 사용자가 지정할 alias·inbound-control 권한을 보여준다.
4. 사용자가 양쪽에서 code를 확인한다. 한쪽 확인만 끝났거나 시간이 만료되면 trust는 생기지 않는다.
5. 양쪽 signed receipt가 검증된 뒤 각 장비가 상대 transport fingerprint를 자기 trust store에 저장한다.
6. 연결은 trust 저장 성공 뒤 별도 동작으로 시작한다. pairing 성공이 active control을 자동 시작하지 않는다.

## 현재 CLI 사용 순서

두 Windows 장비에서 resident agent를 실행한 뒤 다음 순서로 진행한다. 아래 `A_PRIVATE_IP`,
`PAIR_PORT`, `CONTROL_PORT`, `OWN_JOB_ID`, `CODE`는 실제 값으로 바꾼다. 포트는 명시적으로
선택하며 방화벽 규칙을 자동으로 변경하지 않는다. 현재 한 장비당 활성 화면 하나를 지원한다.

1. A에서 `krasis pair listen B A_PRIVATE_IP:PAIR_PORT --allow-inbound-control`을 실행한다.
2. B에서 `krasis pair dial A A_PRIVATE_IP:PAIR_PORT --allow-inbound-control`을 실행한다.
3. 각 장비에서 반환된 **자기 장비의** 작업 ID로 `krasis pair status OWN_JOB_ID`를 조회한다.
   `confirmation_required`가 되면 양쪽 화면의 6자리 코드를 직접 비교한다.
4. 코드가 같을 때만 각각 `krasis pair confirm OWN_JOB_ID CODE`를 실행한다.
   다르면 `krasis pair cancel OWN_JOB_ID`로 중단한다. 코드·작업 ID를 다른 사람에게 전달해
   대신 승인하게 하지 않는다. 양쪽 status가 `pairing_completed`인지 확인한다.
5. A에서 `krasis connection set B A_PRIVATE_IP:CONTROL_PORT RIGHT`,
   B에서 `krasis connection set A A_PRIVATE_IP:CONTROL_PORT LEFT`를 실행한다.
   예시는 B가 A의 오른쪽에 있는 배치다. A는 수신하고 B는 A에 접속한다.
6. A에서 `krasis connect B`, B에서 `krasis connect A`를 실행한 뒤 `krasis status`로
   확인한다. 종료는 양쪽에서 `krasis disconnect`를 실행한다.

페어링 전용 TLS 채널은 입력을 전달하지 않는다. 초기 인증서의 신뢰는 양쪽 사용자 코드 확인과
서명된 receipt로 확정한다. 이후 입력 연결은 인증서 체인·이름·지문을 검사하는 별도 mutual TLS다.
작업은 5분 뒤 만료하며 실패하면 자동으로 신뢰를 승인하거나 재시도하지 않는다.
한쪽 저장만 성공할 수 있으므로 양쪽 결과가 다르면 연결하지 말고 성공한 쪽의 신뢰부터 철회한다.
같은 인증서의 재등록 및 부분 실패 자동 복구는 아직 지원하지 않는다.

### 기존 장비 교체

먼저 양쪽에서 `krasis disconnect`를 실행한다. 신뢰를 교체할 장비에서 `krasis trust list`로
기존 지문을 확인하고, 새 페어링 시작 명령 뒤에 `--replace OLD_FINGERPRINT`를 붙인다.
예: `krasis pair listen NEW_ALIAS IP:PORT --allow-inbound-control --replace OLD_FINGERPRINT`.
`pair dial`에도 같은 옵션을 사용할 수 있다. 옵션은 위 순서를 따른다.

기존 지문은 **이 명령을 실행하는 장비의 trust 목록**에 있어야 한다. 새 장비는 기존 신뢰가 없으면
일반 listen/dial 명령을 사용한다. 양쪽의 확인 코드를 비교하고 각각 confirm한 뒤 status를 확인한다.
교체 대상으로 지정한 이전 장비는 성공 시 철회되며, 그 장비의 연결 설정은 새 지문으로 자동 이전하지
않는다. 새 alias에 `connection set`을 다시 실행해야 한다.

이미 철회한 지문, 없는 지문, 수신/접속 역할이 반대인 대상은 교체할 수 없다. 승인 대기 중 기존
지문이 철회되어도 최종 저장은 실패한다. 이 절차는 다른 인증서를 가진 새 peer로 교체하는 기능이며,
같은 인증서 재승인이나 로컬 인증서 갱신 기능은 아니다. 부분 성공 시 양쪽 trust를 다시 확인한다.

### 로컬 인증서 갱신

transport 인증서는 30일이며 자동 갱신하지 않는다. `krasis credential show LISTENER` 또는
`krasis credential show CONNECTOR`로 현재 지문·역할·만료를 조회한다. 조회는 새 인증서를 만들지 않는다.
갱신은 `krasis credential renew ROLE EXPECTED_LOCAL_FINGERPRINT`로 실행한다. 여기서 ROLE은
LISTENER 또는 CONNECTOR이며, 지문은 **로컬 show 결과**다. 만료된 인증서도 갱신할 수 있지만,
손상되었거나 기기 신원이 바뀐 인증서는 덮어쓰지 않는다. 갱신 뒤 상대 신뢰는 자동 변경되지 않는다.

현재 지원 절차는 양쪽 갱신이다. A가 LISTENER, B가 CONNECTOR라면:

1. 양쪽 연결·페어링을 중지하고 현재 설정과 양쪽 역할의 기존 지문을 기록한다.
2. A는 LISTENER, B는 CONNECTOR를 각각 위 명령으로 갱신한다.
3. A는 `pair listen ... --replace B_OLD_FINGERPRINT`, B는
   `pair dial ... --replace A_OLD_FINGERPRINT`로 재페어링한다. 앞 절의 전체 인자와 양쪽 확인 절차를 따른다.
4. 양쪽 완료를 확인하고 새 신뢰에 connection set을 다시 실행한 뒤 연결한다.

한쪽만 갱신한 경우 동일 상대 인증서 재등록이 막힐 수 있다. 자동 재승인이나 파일 삭제로 우회하지
않고 반대쪽도 해당 역할을 갱신한 뒤 위 교체 절차를 따른다. 다른 peer도 같은 역할 인증서를 신뢰했다면
그 peer와의 연결도 끊기므로 각 peer의 신뢰를 별도로 교체해야 한다. 응답 유실 시 show로 지문을 다시
확인한다. 이전 지문으로 재시도해 인증서가 연속 회전하는 것은 차단한다.

heartbeat 250ms,
peer timeout 2초, 화면 복귀 재무장 거리 32px는 초기 설정이며 실기기 지연 보장값이 아니다.
로컬 테스트는 CLI 페어링과 TLS 연결을 검증하지만 실제 두 PC 입력, secure desktop,
관리자 권한 앱 동작을 검증한 것은 아니다.

## 이름과 권한 변경

alias와 inbound-control capability는 active fingerprint를 대상으로 같은 trust revision에서 변경한다.
권한 제거는 다음 lease authorization부터 적용한다. 현재 active lease는 authenticated management API로
정확히 종료한 뒤 변경한다.

## 회전과 교체

private key 노출 의심, 장비 분실·양도 또는 명시적 rotation은 새 generation을 만든다. 다른 장비에서는
이전 transport fingerprint 폐기와 새 fingerprint 등록을 하나의 atomic replacement로 수행한다. 단순
alias 변경이나 로컬 파일 삭제는 revocation이 아니다.

## Phase 0 자료

`krasis-lab peer-credentials` bundle과 수동 `trust` directory는 자동 가져오지 않는다. 양쪽 실험 process를
중지하고 active control이 없는지 확인한 뒤 기존 fingerprint를 폐기하고 실험 secret directory를 삭제한다.
그 다음 제품에서 새 identity와 pairing을 만든다. 동기화 drive나 공유 폴더를 migration 경로로 사용하지
않는다.
