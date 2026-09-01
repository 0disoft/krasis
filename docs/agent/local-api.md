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
| `GET` | `/v1/status` | identity fingerprint/generation, `LOCAL|CONNECTED`, redacted alias, allowed actions |
| `POST` | `/v1/connect` | configured shared-core session runner 시작 |
| `POST` | `/v1/disconnect` | active runner cancel과 최대 5초 wait |
| `POST` | `/v1/pair` | `BEGIN`, `CONFIRM`, `CANCEL` product pairing workflow |
| `GET` | `/v1/diagnostics` | stable code별 aggregate counter와 마지막 state |
| `POST` | `/v1/safe-stop` | agent root cancellation 요청 |

mutation body schema는 `krasis.agent.operation/v1`, 결과는
`krasis.agent.operation-result/v1`이다. 모든 body는 64KiB 이하 strict JSON이다. `connect`는 alias만,
`disconnect`는 추가 field 없이, pair action은 해당 action에 필요한 alias/capability 또는
pairing ID/code만 허용한다.

`status`와 diagnostics는 private key, certificate body, transport address, target token, session·lease·epoch,
raw input, path와 internal error를 반환하지 않는다. active-lease exact revoke operation은
`docs/agent/management-contract.md` 계약을 이 transport 위에서 사용한다.

## 안정 코드

현재 process owner의 lifecycle code는 `ready`, `connect_started`, `already_active`, `not_configured`,
`no_active_session`, `disconnected`, `shutdown_timeout`, `session_stopped`, `safe_stop_requested`다.
HTTP adapter 자체는 `unauthenticated`, `invalid_request`, `method_not_allowed`, `not_found`,
`internal_failure`를 반환한다. #14는 이를 CLI exit taxonomy에 매핑한다.
