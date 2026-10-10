# ADR 0070: 기기 승인과 연결 인증서 수명 분리

- 상태: Accepted; g263 implemented, paired reboot acceptance pending
- 결정일: 2026-10-11
- 관련: ADR 0036, ADR 0069

## 결정

한 번 승인한 기기 연결은 사용자가 철회할 때까지 유지한다. 인증서 전체의
지문을 매달 새로 페어링하는 대신, 페어링으로 확인한 역할별 Ed25519 공개키를
신뢰 대상으로 사용한다. 기존 trust record의 fingerprint와 승인한 DER는
그대로 유지해 설정·배치·클립보드·업데이트 권한의 기기 참조가 바뀌지 않는다.

이 경로는 제품의 `NewProductKeyTLSConfig`와 명시적인 key anchor가 있는
network source에서만 사용한다. 기존 CA 기반 TLS와 pairing confirmation은
기존 인증서 유효 기간·verified-chain 검사를 유지한다. 초기 페어링은 반드시
유효한 인증서로 수행하며, 만료된 인증서를 새 기기 승인에 쓰지 않는다.

제품 TLS는 TLS 1.3, 세션 재개 금지, 정확히 하나의 leaf, 고정 제품 DNS,
정확한 server/client 역할, Ed25519 키, self-signature, 현재 시각의 유효 기간,
30일 수명 상한과 승인한 SPKI의 일치를 확인한다. client의 custom verifier는
일반 chain 검증을 이 명시적인 검증으로 대체한다. transport session도
handshake 이후 같은 key anchor를 별도로 검증하고 원래 pairing principal을
반환한다. 관리 HTTP 요청과 입력 lease는 현재 trust·철회·권한을 다시 검사한다.

## 자동 인증서 갱신

저장된 역할 키와 원래 인증서를 검증하고, 잔여 수명이 7일 이하이면 같은 키로
30일 유효 인증서를 메모리에서 발급한다. 23일 간격의 유효 기간 창을 사용해
연속 재접속·정책 조회가 수명을 임의로 계속 미루지 않게 한다. 오래 종료됐다
실행되거나 로그인 전에 인증서가 만료된 경우에도 현재 창의 새 인증서를
제시할 수 있다. 실제로 제시한 만료 인증서는 항상 거절한다.

새 인증서는 다음 TLS handshake에서 선택한다. 이미 인증한 연결은 같은 기기
신뢰가 유지되는 동안 계속 사용하며, 새 handshake에는 새 인증서가 필요하다.
peer 철회와 신뢰 조회 실패의 기존 감시·정리 정책은 유지한다. 보호 입력 lease는
별도로 최대 55분으로 제한하고 서비스의 기존 expiry·heartbeat 검사를 유지한다.

사용자 credential 파일이나 ACL을 갱신하지 않는다. 서비스도 보호된 승인으로
선택한 기존 프로필을 읽기만 하므로 로그인 전 자동 갱신에 사용자 credential
파일 쓰기 권한이나 새로운 인증서 저장소 등록이 필요하지 않다. 역할 키 교체는
기존 명시적 `credential renew` 명령으로 수행하며 자동 갱신과 구분한다.
그 명령은 새 키를 만들므로 재페어링과 로그인 전 재승인이 필요하다.

## 로그인 전 승인

새 `krasis.login-grant/v2`는 expiresAt 없이 관리자가 저장한다. local transport
pin은 SPKI SHA-256이며 원래 사용자 identity·peer fingerprint·역할·inbound
허용 상한을 고정한다. runtime은 현재 프로필 정책과 승인한 키를 읽는다.
자동 시작 해제, 수동 연결 해제, 별칭/역할/키 변경, trust 철회, 손상·pending
승인은 연결을 허용하지 않는다. 사용자 SID·private key는 진단에 추가하지 않는다.

기존 v1 승인은 기존 만료와 인증서 pin을 유지한다. 기한 있는 승인을 자동으로
영구 승인으로 승격하지 않는다. 양쪽 g263 적용 후 각자의 기존 peer 별칭으로
`krasis-install --prepare-login PEER`를 관리자 확인하여 한 번 전환한다.
이후 인증서 갱신이나 버전 업데이트에는 반복 승인이 필요하지 않다.
`--disable-login`은 두 승인 schema를 모두 해제한다.

승인 실행은 설치 결과와 별도로 `login-configuration-latest.json`에 실패 단계,
종료 결과와 확보한 Win32 숫자를 저장한다. stdout을 수집하지 못해도 새 support의
`loginConfiguration`으로 전송할 수 있다. 사용자 SID·별칭·프로필 경로·개인키·
원시 오류 문자열은 이 진단에 포함하지 않는다. 이전 설치 성공 receipt를 이번
승인 성공으로 해석하지 않는다. 승인 저장 실패는 설정을 바꾸거나 설치 transaction을
삭제해 우회하지 않고, 단계와 현재 정책을 확인한 다음 처리한다.

## 호환성과 검증

g262는 새 leaf의 동일 키 증명을 처리하지 못하므로, 갱신이 시작되기 전에
양쪽 g263을 적용해야 한다. 자동 배포는 기존 agent가 제공하는 관리 채널로
새 묶음을 먼저 전달하고 양쪽 교체를 완료한다. 서로 다른 버전의 입력 성공을
전제로 승인하지 않는다. 초기 페어링의 cert pin은 명시적 확인 결과를 보존한다.

만료 90일 뒤 재접속, 실 TLS handshake에서 원래 principal 유지, 다른 키 거절,
만료 leaf 거절, persistent grant roundtrip·legacy expiry, 현재 trust 철회,
local key 교체와 55분 입력 lease를 검증한다. 실제 재부팅과 LogonUI 입력은
양쪽 설치·승인 전환 뒤 별도로 확인한다.
