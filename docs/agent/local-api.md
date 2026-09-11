# Resident agent local API

- 상태: Active contract
- 결정: `docs/adr/0036-resident-agent-and-local-api-ownership.md`
- transport: IPv4 loopback HTTP with protected per-process bearer token

## 발견과 인증

agent는 `%LOCALAPPDATA%\Krasis\control\endpoint.json`에 `krasis.local-api-endpoint/v1` 주소를,
`auth-token`에 64자리 lowercase hex token을 기록한다. 두 file은 current user와 `SYSTEM` 전용이며
agent 종료 때 삭제된다. client는 두 file을 strict하게 읽고 `Authorization: Bearer TOKEN`을 보낸다.

주소는 `127.0.0.1:1..65535`만 허용한다. API를 LAN에 bind하거나 token을 command line, log, report,
clipboard, environment 또는 UI projection에 복제하지 않는다.

## Endpoint

| Method | Path | Operation |
| --- | --- | --- |
| `GET` | `/v1/status` | identity fingerprint/generation, 연결 lifecycle state, redacted alias, allowed actions |
| `POST` | `/v1/connect` | configured shared-core session runner 시작 |
| `POST` | `/v1/connection` | 신뢰된 peer의 연결 설정 저장 또는 조회; 세션 시작과 분리 |
| `POST` | `/v1/disconnect` | active runner cancel과 최대 5초 wait |
| `POST` | `/v1/pair` | `BEGIN`, `CONFIRM`, `CANCEL` product pairing workflow |
| `GET` | `/v1/diagnostics` | stable code별 aggregate counter와 마지막 state |
| `GET` | `/v1/trust` | fingerprint, alias, role, inbound capability와 revoked 상태의 redacted projection |
| `POST` | `/v1/trust/revoke` | inactive trusted fingerprint의 atomic local revocation |
| `GET` | `/v1/topology` | 연결 상태와 non-revoked peer 목록; 저장된 배치가 없으면 `UNCONFIGURED` |
| `POST` | `/v1/safe-stop` | agent root cancellation 요청 |

connection을 제외한 mutation body schema는 `krasis.agent.operation/v1`, 결과는
`krasis.agent.operation-result/v1`이다. 모든 body는 64KiB 이하 strict JSON이다. `connect`는 alias만,
`disconnect`는 추가 field 없이, pair action은 해당 action에 필요한 alias/capability 또는
pairing ID/code만 허용한다. trust revoke는 lowercase SHA-256 fingerprint 하나만 허용한다.

`status`와 diagnostics는 private key, certificate body, transport address, target token, session·lease·epoch,
raw input, path와 internal error를 반환하지 않는다. active-lease exact revoke operation은
`docs/agent/management-contract.md` 계약을 이 transport 위에서 사용한다.

## 안정 코드

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

## 연결 설정

제품 CLI는 `krasis connection set ALIAS IP:PORT LEFT|RIGHT`와
`krasis connection show ALIAS`를 제공한다. 두 명령 모두 실행 중인 agent의 인증된 API를 사용한다.
`--json`은 `krasis.agent.connection/v1` 결과를 그대로 출력한다.

`/v1/connection` 요청은 `schemaVersion: krasis.agent.connection/v1`, `action: SET|SHOW`,
`peerAlias`를 받는다. SET에는 `address`와 `edge`가 필요하고 SHOW에는 둘 다 허용하지 않는다.
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

설정 저장은 페어링이나 연결 시작을 뜻하지 않는다. 현재 기본 제품 runner와 pairing adapter 연결은
아직 미완료이며, 설정만 저장해도 connect가 활성화되지는 않는다. topology의 배치 projection 역시
실행기 연결 전까지 `UNCONFIGURED`를 유지한다. LAN 주소는 이 명시적 설정 조회에만 노출하고
status와 diagnostics에는 추가하지 않는다.
