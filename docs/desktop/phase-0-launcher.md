# Phase 0 Native Launcher

- 상태: Experimental
- 대상: Windows 11 x64 Phase 0 실기 테스트
- 실행 파일: `krasis-launcher.exe`

## 목적

두 노트북에서 긴 `agent-session` 명령을 반복 입력하지 않고 Pair 준비, 연결 시작, 안전 중지와 이번 실행
보고서 복사를 버튼으로 수행한다. 이 launcher는 정식 tray 앱이나 설치 프로그램이 아니다.

현재 v21 queue/disposition 실기 검증용 로컬 package는 configured one-shot intent
`krasis_probe_kit_package_v21_queue_disposition_r2`가 생성하는
`dist/krasis-probe-kit-v21-queue-disposition-r2-windows-amd64.zip`이다. 이 파일은 코드 서명된 release가
아니며 두 노트북에 같은 ZIP을 복사해 각각 새 폴더에 풀어 사용한다. 기존 v19 kit와 섞지 않는다.

시작할 때 `SHA256SUMS.txt`의 고정 package entry와 자신의 executable을 포함한 모든 배포 file을 검사한다.
누락, manifest drift 또는 hash 불일치가 있으면 Pair와 연결 버튼을 잠근다. 실행 뒤 생성되는
`launcher-config.json`, `reports`, `secrets`는 배포 checksum 대상이 아니므로 다음 실행을 막지 않는다.
이 검사는 manifest와 artifact가 함께 교체된 경우를 막는 코드 서명은 아니다.

처음 실행의 `HP15 IP`는 비어 있다. 저장소나 배포물에 특정 집·스터디카페 주소를 기본값으로 넣지 않으며,
사용자가 현재 listener의 실제 LAN IP를 입력하기 전에는 설정 저장, Pair 생성과 연결 시작이 실패한다.

## 소유 경계

- `krasis-launcher.exe`는 역할·주소·상대 경로 설정과 child process 수명만 소유한다.
- 실제 hook, 입력 억제·전달·주입, mTLS, lease, crash guard와 aggregate report는 계속
  `krasis-lab.exe`가 소유한다.
- launcher는 `krasis-lab.exe`를 숨겨진 child process로 시작하지만 강제 종료하지 않는다. `안전 중지`와
  창 닫기는 현재 run 전용 stop file을 만들고 기존 agent cleanup을 기다린다.
- 한 번에 하나의 Pair 준비 또는 agent session만 실행한다.

## 버튼 동작

- `설정 저장`: executable 옆 `launcher-config.json`을 교체한다. private key나 report 내용은 저장하지 않는다.
- `새 Pair 생성`: Listener에서 새 단기 credential pair를 생성하고 listener를 보호·신뢰 등록한 뒤 복사할
  `connector` directory를 Explorer로 연다. 새로 푼 kit에 `secrets`가 없으면 launcher가 먼저 만들고 현재
  Windows 사용자와 `SYSTEM` 전용 ACL로 잠근다. 실제 `pair-*` directory는 CLI가 배타적으로 생성한다.
- `Connector 준비`: 사용자가 복사한 connector directory를 현재 Windows 사용자에게 다시 잠그고 peer를
  trust store에 등록한다.
- `연결 시작`: timestamp가 들어간 새 agent/guard report와 stop-file 경로를 고정한 뒤 bounded session을
  시작한다. 기존 report를 덮어쓰지 않는다. credential chain이 설정한 실행 시간의 종료 시각까지
  유효하지 않으면 `credential_lifetime_insufficient` report를 남기고 연결하지 않으므로 새 Pair를 준비한다.
- `안전 중지`: process kill 대신 stop-file cancellation을 요청한다. child가 정리되기 전 다른 작업을
  시작할 수 없다.
- `보고서 복사`: 이번 launcher 실행이 실제로 만든 agent/guard JSON만 하나의
  `krasis.launcher.report-bundle/v1` object로 클립보드에 복사한다. file path와 credential은 넣지 않는다.
- `폴더 열기`: 설정된 report directory를 Explorer로 연다.

## 실패와 복구

launcher가 비정상 종료돼도 `krasis-lab.exe`의 lifecycle owner와 crash guard가 안전 수렴을 담당한다.
launcher 창을 정상적으로 닫으면 stop-file을 요청하고 child 종료를 기다린다. stop-file 생성이 실패하면 창을
닫지 않고 오류를 표시한다. child exit code가 0이 아니어도 agent와 guard report가 둘 다 존재하면 복사
버튼을 열어 실패 증거를 보존한다.

launcher 재시작은 이전 run report를 자동 선택하지 않는다. 오래된 보고서를 최신 결과로 오인하는 것보다
명시적으로 Explorer에서 전달하는 쪽을 택한다. launcher binary 자체가 시작되지 않으면 같은 kit의 수동
CLI 절차로 돌아갈 수 있다.

## 실험 종료 뒤 정리

1. 양쪽 launcher에서 현재 run이 종료되고 `안전 중지`가 더 이상 활성 상태가 아닌지 확인한다.
2. 필요한 agent/guard report를 `보고서 복사` 또는 Explorer로 보존한다. 실패 report도 원인 분석이
   끝나기 전에는 지우지 않는다.
3. 이번 pair가 더 필요하지 않으면 HP15와 gram13 양쪽에서 해당 `secrets/pair-*` 또는 복사한 역할
   directory를 삭제한다. 24시간 certificate 만료와 `trust revoke`는 private key file을 지우지 않는다.
4. report 분석이 끝나면 각 kit의 `reports` directory를 사용자가 삭제할 수 있다. launcher는 다른 장비의
   사용 상태와 evidence 보존 여부를 알 수 없으므로 자동 cleanup하지 않는다.

## 아직 제공하지 않는 것

- 설치, 코드 서명, tray 상주, 자동 시작과 자동 업데이트
- 방화벽 규칙 자동 변경
- credential 자동 전송 또는 장비 발견
- credential·report 일괄 자동 삭제와 secure deletion
- 세 대 이상, 다중 모니터와 임의 토폴로지
- report upload, 원격 측정 또는 클라우드 계정
