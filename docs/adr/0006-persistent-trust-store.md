# ADR 0006: Persistent Peer Trust Store

- 상태: Accepted
- 결정일: 2026-07-13
- 범위: Phase 0 CLI trust persistence와 향후 제품 pairing 저장 경계

## 배경

`agent-session`은 `peer.pem`에서 만든 process-local grant만 사용했다. fingerprint, 역할,
inbound control 권한과 폐기 상태를 검사했지만 process 재시작 뒤에는 승인과 폐기가 남지 않았다.
CA가 발급한 인증서라는 사실만으로 제어권을 허용할 수 없으므로 peer별 사용자 신뢰가 durable
authority로 남아야 한다.

## 결정

- trust store schema는 `krasis.trust-store/v1` JSON document다.
- store directory는 `trust.json`과 persistent `.trust.lock`만 소유한다.
- record는 certificate SHA-256 fingerprint, EKU 기반 역할, 사용자 alias,
  `allowInboundControl`, 생성 시각과 선택적 폐기 시각만 저장한다.
- certificate 원문, private key, 주소, session·epoch·lease와 원시 입력은 저장하지 않는다.
- unknown field, trailing JSON, 중복 fingerprint, 잘못된 alias·역할·시각, 1MiB 초과와 schema
  불일치는 전부 fail-closed한다. 손상된 store를 빈 allowlist로 조용히 대체하거나 자동 복구하지 않는다.
- mutation은 process 간 non-blocking exclusive lock 안에서 revision을 증가시키고, 같은 directory의
  제한된 temporary file을 flush한 뒤 atomic replace한다. Windows replace는 write-through를 사용한다.
- Windows store의 부모 directory, store directory, lock과 document는 현재 사용자와 `SYSTEM`만
  허용하는 protected DACL을 사용하고 load 전에 DACL과 reparse path를 검증한다.
- `trust import`는 `peer.pem`을 fingerprint·역할 입력으로 사용하지만 certificate 원문을 store에
  복제하지 않는다. 같은 fingerprint 재등록과 폐기된 fingerprint의 부활은 거부한다.
- `trust revoke`는 record를 삭제하지 않고 폐기 시각을 기록한다.
- `agent-session`의 시작 시점 immutable grant snapshot 결정은 ADR 0032가 대체한다. store 형식과
  mutation 원자성, fingerprint·역할·capability·폐기 record 결정은 이 ADR에 그대로 남는다.

## 결과

장점은 재시작 뒤에도 승인과 폐기가 남고 runtime authorization이 파일 I/O에서 분리된다는 점이다.
단점은 정식 사용자 승인 UI, key rotation, alias 변경, backup·migration UX와 product default data
directory가 아직 없다는 점이다. Phase 0에서는 명시적인 `--store DIRECTORY`만 사용한다.

## 검증

- import, list, revoke, duplicate와 repeated revoke
- corrupt·unknown·trailing·oversized JSON의 fail-closed load
- permissive directory와 lock contention 거부
- atomic replace 뒤 revision persistence와 temporary file cleanup
- store record에서 runtime grant snapshot 조립
