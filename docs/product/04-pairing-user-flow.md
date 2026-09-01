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
