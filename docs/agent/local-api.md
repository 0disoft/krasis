# Resident agent local API

- 상태: Active contract
- 결정: `docs/adr/0036-resident-agent-and-local-api-ownership.md`
- transport: IPv4 loopback HTTP with protected per-process bearer token

## 발견과 인증

diagnostics counter의 `log_incomplete = 1`은 파일 기록 실패 또는 로그 큐 초과를 뜻한다.
진단 로그는 로컬 파일이며 API 응답 schema와 기기 신뢰 저장 형식은 변경하지 않는다.

agent는 `%LOCALAPPDATA%\Krasis\control\endpoint.json`에 `krasis.local-api-endpoint/v1` 주소를,
`auth-token`에 64자리 lowercase hex token을 기록한다. 두 file은 current user와 `SYSTEM` 전용이며
agent 종료 때 삭제된다. client는 두 file을 strict하게 읽고 `Authorization: Bearer TOKEN`을 보낸다.

주소는 `127.0.0.1:1..65535`만 허용한다. API를 LAN에 bind하거나 token을 command line, log, report,
clipboard, environment 또는 UI projection에 복제하지 않는다.

## Endpoint

| Method | Path | Operation |
| --- | --- | --- |
| `GET` | `/v1/status` | identity fingerprint/generation, 연결 lifecycle state, redacted alias, allowed actions, 안전한 화면 변경 종료 시 `stopReason: display_change`, 선택된 입력 경로 `inputBackend` |

g138의 선택적 `inputBackend`는 runner가 실제 선택한 `PROTECTED` 또는 `PORTABLE`이다.
세션 종료·새 연결 시작 전에는 `UNKNOWN`이며, 이전 agent의 필드 생략도 확인 불가로
처리한다. 이 표시는 서비스 설치·현재 lease 준비·PIN 입력 성공을 보증하지 않으며
입력 권한 판단에 사용하지 않는다. portable 경로의 PIN·관리자 창 제한을 UI에 표시한다.
| `GET` | `/v1/runtime` | schema, protocol, agent generation, build revision, process instance nonce |
| `POST` | `/v1/upgrade-stop` | empty body와 `If-Match` instance nonce를 사용하는 idle-only 종료 |
| `POST` | `/v1/connect` | configured shared-core session runner 시작 |
| `POST` | `/v1/connection` | 신뢰된 peer의 연결 설정 저장 또는 조회; 세션 시작과 분리 |
| `POST` | `/v1/credential` | 역할별 로컬 인증서 공개 정보 조회 또는 예상 지문을 지정한 명시적 갱신 |
| `POST` | `/v1/disconnect` | active runner cancel과 최대 5초 wait |
| `POST` | `/v1/pair` | `LISTEN`, `DIAL`, `STATUS`, `CONFIRM`, `CANCEL` product pairing workflow |
| `GET` | `/v1/diagnostics` | stable code별 aggregate counter와 마지막 state |
| `POST` | `/v1/diagnostic-collection` | 신뢰된 peer 지문으로 양쪽 정제 진단 수집 START, 요청 ID별 STATUS |
| `POST` | `/v1/app-open` | 신뢰된 peer 지문으로 마지막 UI 열기 START, 요청 ID별 STATUS |
| `GET` | `/v1/trust` | fingerprint, alias, role, inbound capability와 revoked 상태의 redacted projection |
| `POST` | `/v1/trust/revoke` | inactive trusted fingerprint의 atomic local revocation |
| `GET` | `/v1/topology` | 연결 상태와 non-revoked peer 목록; 저장된 배치가 없으면 `UNCONFIGURED` |
| `POST` | `/v1/safe-stop` | agent root cancellation 요청 |

upgrade-stop은 인증 후 빈 body와 현재 instance nonce를 검사한다. nonce 불일치는 409로 거부한다.
선택적인 `X-Krasis-Upgrade-Generation`과 `X-Krasis-Upgrade-Build`는 함께 전달하며,
형식 오류는 400으로 거부한다. 업데이트 적용 중에는 두 값이 수신·검증된 작업의 세대와
40자리 빌드 리비전에 정확히 일치할 때만 기존 에이전트를 종료한다. 연결·페어링 중에는
agent 잠금 안에서 종료를 거부한다. 일반 safe-stop과 별개인 자동 교체 전용 경로다.
runtime schema는 `krasis.agent.runtime/v1`이며 기존 status/result schema는 변경하지 않는다.

g109의 `/v1/app-open`은 `{action: "START", peer: FINGERPRINT}` 또는
`{action: "STATUS", peer: FINGERPRINT, id: REQUEST_ID}`만 받는다. 경로나 실행 인자를
받지 않는다. 관리 채널의 `/v1/app-open`은 등록된 TLS 상대에게만 동일 요청 ID를 전달한다.
수신자는 매 요청에서 신뢰와 inbound-control 권한을 확인하고, 자신의 보호된 실행 기록과
SHA-256을 검증한 후 일반 사용자 세션에서 UI를 연다. 요청은 1분 후 만료되며 반복 요청은
2분간 중복 실행하지 않는다. `launch_requested`는 실행 요청 성공만 의미한다.

connection과 upgrade-stop을 제외한 mutation body schema는 `krasis.agent.operation/v1`, 결과는
`krasis.agent.operation-result/v1`이다. 모든 body는 64KiB 이하 strict JSON이다. `connect`는 alias만,
`disconnect`는 추가 field 없이, pair action은 해당 action에 필요한 alias/capability 또는
pairing ID/code만 허용한다. trust revoke는 lowercase SHA-256 fingerprint 하나만 허용한다.
pair LISTEN/DIAL은 alias, 명시적 inbound capability와 private/loopback `address`가 필요하다.
CLI의 `--allow-inbound-control`과 `--deny-inbound-control`은 각각 `allowInboundControl: true`,
`false`를 전달한다. 둘 중 하나만 지정해야 하며 false는 필드 생략과 다르다. 서버는 기존과 같이
명시적인 boolean을 요구하고, 재확인 시 저장된 권한과의 정확한 일치를 검사한다. 새 endpoint나
schema 변경은 없다. 레거시 BEGIN의 지원 범위는 변경하지 않는다.
STATUS/CANCEL은 pairing ID만, CONFIRM은 pairing ID와 6자리 code만 받는다.
LISTEN/DIAL의 선택적 `fingerprint`는 사용자가 교체를 승인한 기존 peer의 lowercase SHA-256 지문이다.
지문을 생략하면 신규 등록이며, 지정하면 작업 생성 시 존재·철회·역할을 확인하고 작업에 고정한다.
CONFIRM에서 교체 대상을 바꿀 수 없다. 활성 입력 세션 중에는 작업 생성과 확인을 거부하며,
최종 저장 시 기존 지문의 철회 상태를 다시 확인한다. 잘못된 역할은 `replacement_role_mismatch`,
없는 대상은 `peer_not_found`, 철회된 대상은 `peer_already_revoked`로 거부한다.
store 조회 실패는 `FAILED/trust_store_unavailable`이며 자동 승인하지 않는다.
기존 BEGIN 요청은 기본 네트워크 adapter에서 지원하지 않는다.

`status`와 diagnostics는 private key, certificate body, transport address, target token, session·lease·epoch,
raw input, path와 internal error를 반환하지 않는다. active-lease exact revoke operation은
`docs/agent/management-contract.md` 계약을 이 transport 위에서 사용한다.
별도 `/v1/diagnostic-collection`은 인증된 로컬 CLI에만 두 저장 파일의 경로를 반환한다.
START는 이 PC의 로그를 내보내고 인증된 상대에게 1분짜리 요청을 건다. STATUS는 같은 지문과
32자리 요청 ID가 일치할 때만 `PENDING`, `RECEIVED`, `EXPIRED`와 경로를 반환한다.

## 안정 코드

페어링 결과의 기존 `code` 필드에 `pairing_commit_uncertain`을 추가한다(`status: FAILED`).
이는 로컬 최종 저장을 호출했지만 오류가 반환되어 영속 결과를 확정할 수 없다는 뜻이다.
`pairing_failed`, `pairing_expired`, `pairing_cancelled`는 해당 작업의 로컬 최종 저장 미시도이며
기존 trust 부재를 보장하지 않는다. `pairing_completed`는 로컬 성공만 보장한다. 모든 경우 상대
저장 결과는 별도로 상대 status를 조회해야 한다. 새 JSON 필드·endpoint·wire 메시지는 추가하지 않는다.
STATUS는 기존 작업의 결과를 변경하지 않는다. 작업 교체·process 재시작 뒤 `pairing_not_found`는
저장 실패의 증거가 아니다. 복구 절차는 `docs/product/04-pairing-user-flow.md`를 따른다.

연결 상태는 `LOCAL`, `CONNECTING`, `CONNECTED`, `RECONNECTING`, `DISCONNECTING`이다.
runner 시작만으로 CONNECTED가 되지 않는다. shared supervisor의 인증된 연결 획득 알림이 있어야
CONNECTED로 표시하며, 입력 제어권 획득을 뜻하지는 않는다. 재시도 대기와 재접속은 RECONNECTING,
취소부터 runner cleanup 완료까지는 DISCONNECTING이다. status와 topology가 같은 상태를 사용한다.
이전 runner가 늦게 보낸 상태는 새 runner에 적용하지 않는다. 취소되지 않은 runner의 오류는
원문 대신 `session_failed`로 기록한다. 종료된 agent에 새 connect를 요청하면 `operation_cancelled`다.

현재 process owner의 lifecycle code는 `ready`, `connect_started`, `already_active`, `not_configured`,
`no_active_session`, `disconnected`, `shutdown_timeout`, `session_stopped`, `trust_revoked`,
`active_session_requires_disconnect`, `peer_not_found`, `peer_already_revoked`, `trust_store_busy`,
`safe_stop_requested`다.
HTTP adapter 자체는 `unauthenticated`, `invalid_request`, `method_not_allowed`, `not_found`,
`internal_failure`를 반환한다. 제품 CLI는 이를 `docs/cli/output-and-exit-codes.md`의 exit taxonomy에
매핑한다.

## 로컬 인증서 관리

인증된 `POST /v1/credential`은 `schemaVersion: krasis.agent.credential/v1`, `action: SHOW|RENEW`,
`role: LISTENER|CONNECTOR`를 받는다. RENEW에는 `expectedFingerprint`가 필수이며 SHOW에는
허용하지 않는다. 64KiB strict JSON과 기존 bearer 인증을 적용한다.

결과는 같은 schemaVersion, status, code와 성공 시 `info`를 포함한다. info는 fingerprint, role,
notBefore, notAfter, expired만 포함하며 private key와 certificate bytes는 노출하지 않는다.
SHOW는 인증서를 생성하지 않고 만료된 인증서도 조회한다. 없으면 `credential_not_found`다.
RENEW는 현재 지문 불일치 시 `credential_changed`, 활성 세션은 `active_session_requires_disconnect`,
페어링 중에는 `pairing_already_active`로 거부한다. lock 충돌은 `FAILED/credential_store_busy`,
손상·신원 불일치 등은 `FAILED/credential_unavailable`이다.

성공 코드는 `credential_loaded` 또는 `credential_renewed_repair_required`다. 갱신은 해당 역할의
로컬 TLS 키/인증서만 원자 교체하며 기기 신원·다른 역할·상대 신뢰를 변경하지 않는다.
응답 유실 후 이전 지문으로 재시도하면 거부될 수 있으므로 SHOW로 현재 상태를 확인한다.
연결·페어링과 갱신은 같은 agent lock으로 직렬화한다. 한쪽만 갱신한 경우 변경되지 않은 상대는
일반 LISTEN/DIAL로 양방향 재확인하며, 바뀐 상대 지문을 보유한 쪽만 fingerprint 교체를 지정한다.
재확인은 저장된 지문·역할·별칭·inbound 권한·인증서 bytes가 모두 같고 현재 유효할 때만 성공한다.
성공 코드는 기존 `pairing_completed`이며 revision과 연결 설정은 유지한다. 최종 재확인 호출에서
불일치·철회·누락·손상으로 실패하면 `FAILED/pairing_commit_uncertain`으로 종료한다. 이 코드는
신뢰가 저장되었다는 뜻이 아니며, 양쪽 status와 영속 trust 상태를 확인해야 한다.
자동 재승인, 정책 변경, 부분 실패 자동 복구는 수행하지 않는다.

## 연결 설정

제품 CLI는 `krasis connection set ALIAS IP:PORT LEFT|RIGHT`와
`krasis connection show ALIAS`를 제공한다. 두 명령 모두 실행 중인 agent의 인증된 API를 사용한다.
`--json`은 `krasis.agent.connection/v1` 결과를 그대로 출력한다.

`/v1/connection` 요청은 `schemaVersion: krasis.agent.connection/v1`, `action: SET|SHOW`,
`peerAlias`를 받는다. SET에는 `address`와 `edge`가 필요하고 SHOW에는 둘 다 허용하지 않는다.
Generation 5부터 `action: SET_EDGE`도 지원한다. `fingerprint`와 LEFT/RIGHT `edge`를 받고
`address`는 허용하지 않는다. trust-store 잠금 안에서 fingerprint를 비교하고 기존 주소를
보존하여 경계만 저장한다. SET_EDGE는 기존 정상 연결 profile이 필요하며, SET과 마찬가지로
활성 세션 중에는 거부한다. SET/SHOW에는 fingerprint를 허용하지 않는다.
Generation 6부터 `SET_REARM`은 `fingerprint`와 `rearmDistance`(32, 64, 128)를 받으며
address/edge를 금지한다. 기존 profile의 주소·edge를 같은 저장소 잠금 안에서 보존하고,
활성 세션 중에는 거부한다. 다른 action에는 0이 아닌 rearmDistance를 허용하지 않는다.
profile의 선택적 rearmDistance가 없거나 0이면 기존 기본값 32를 적용한다. 명시적인 값이
저장된 profile/응답은 이전 strict-decoding agent/CLI와 호환되지 않을 수 있으므로 실행 파일을
함께 업데이트한다. 기존 profile은 별도 마이그레이션 없이 읽으며 신뢰 문서는 변경하지 않는다.
결과는 같은 schemaVersion과 `status`, `code`, 성공 시 `profile`을 포함한다.
profile은 `krasis.connection-profile/v1`이며 `fingerprint`, `address`, `edge`를 포함한다.

- 상대 role이 LISTENER이면 address는 상대에게 접속할 주소다. 상대가 CONNECTOR이면 로컬에서 수신할 주소다.
- edge는 상대 화면에서의 위치가 아니라 **이 장치에서 상대에게 넘어가는 가장자리**다.
- 주소는 private 또는 loopback IPv4/IPv6와 1~65535 포트만 허용한다. DNS, wildcard,
  공인 IP, zone-qualified/link-local 주소는 이 단계에서 지원하지 않는다.
- 별칭은 철회되지 않은 peer 하나로 해석되어야 한다. 저장은 인증서 지문에 묶이며 별칭 변경으로 이전되지 않는다.
- `%LOCALAPPDATA%\Krasis\trust\connection-FINGERPRINT.json`에 current user/SYSTEM 보호와
  trust-store lock, 원자적 교체를 적용한다. 기존 trust.json 형식이나 입력 허용 권한은 변경하지 않는다.
- 세션이 활성 상태이면 SET을 거부한다. 손상된 설정을 자동 덮어쓰지 않으며 철회된 peer의 설정은 사용할 수 없다.
- 성공 코드는 `connection_saved`, `connection_loaded`, 거부 코드는 `connection_not_configured`,
  `peer_not_found`, `peer_alias_ambiguous`, `active_session_requires_disconnect`다.
  잠금 충돌은 `FAILED/trust_store_busy`다.

설정 저장은 페어링이나 연결 시작을 뜻하지 않는다. 기본 agent는 네트워크 pairing adapter와
Windows shared-core runner를 사용한다. connect는 설정과 인증서를 먼저 검사하며 누락·만료 시
`connection_not_configured` 또는 `credential_unavailable`을 반환한다. 입력 세션과 페어링은
동시에 시작할 수 없다. 종료 중 입력 정리 오류는 `FAILED/session_failed`이며 성공으로 숨기지 않는다.
topology의 화면 배치 projection은 아직 `UNCONFIGURED`다. LAN 주소는 명시적 설정 조회에만
노출하고 status와 diagnostics에는 추가하지 않는다.

페어링 작업은 시작부터 최대 2분, TLS handshake는 최대 5초이며 한 번에 한 작업만 유지한다.
STATUS 조회나 확인 요청으로 유효 시간이 연장되지 않는다. 기존 wire v1 offer의 5분 형식은
호환성을 위해 유지하지만 실제 작업 deadline과 최종 저장 검사는 더 짧은 2분을 적용한다.
작업 결과의 `COMPLETED`는 해당 API 요청의 처리를 뜻한다. 페어링 완료 여부는 반드시
`code: pairing_completed`로 판단한다. 진행 코드는 `pairing_waiting`, `confirmation_required`,
`confirmation_submitted`이며 종료 코드는 `pairing_completed`, `pairing_cancelled`,
`pairing_failed`, `pairing_expired`다. 확인 코드 오류는 `confirmation_rejected`와 함께 작업을
취소한다. 틀린 확인 코드를 제출하면 같은 agent의 신규 LISTEN/DIAL은 30초간
`pairing_retry_later`로 거부한다. 별칭이나 주소를 바꿔도 제한은 유지되며 STATUS/CANCEL은
차단하지 않는다. 제한은 메모리 기반으로, agent 재시작을 견디는 PIN 인증 방어책은 아니다.
원격 저장까지 원자적으로 보장하지 않으므로 양쪽 STATUS를 확인해야 한다.

`PIN_LISTEN`은 별칭과 명시적 inbound capability만 받으며 agent가 임의 PIN을 생성한다.
`PIN_DIAL`은 검색 결과의 private IPv4 address, 초대 ID(`pairingId`), 6자리 `confirmationCode`,
별칭과 capability를 받는다. caller가 LISTEN PIN을 지정하거나 PIN을 legacy DIAL에 싣는 것은 거부한다.
PIN은 기존 인증된 loopback API로만 UI에 전달하고 LAN에는 PAKE 메시지와 키 확인 증명만 보낸다.
PIN 작업은 `pin_waiting`, `pairing_registering`, 기존 종료 코드를 사용하며 수동 CONFIRM을 거부한다.
STATUS에서 `pairing_completed`를 확인하면 기존 기기 키 인증을 사용한다. PIN 모드가 아닌 기존
SAS 작업에는 양쪽 사용자 확인이 여전히 필요하다. 새 기기 등록은 trust 저장 후 연결 설정까지
저장하며 중간 실패는 `pairing_commit_uncertain`으로 보고한다. 양쪽 저장은 원자적이지 않다.
