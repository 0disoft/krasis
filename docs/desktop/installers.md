# Windows 설치

- 상태: 개발용 보호 입력 설치기 구현, 서명된 외부 배포는 #16에 남음
- 기준: ADR 0066, `internal/inputworker`, `cmd/krasis-install`

## 현재 구현

배포 폴더에는 관리 앱 `krasis.exe`, 상주 `krasis-agent.exe`, 터미널용
`krasis-cli.exe`, 보호 입력 설치기 `krasis-install.exe`가 있다. 기본 앱은 portable로
실행하며, 관리자 프로그램·Windows PIN 창 제어에는 선택적인 보호 입력 설치가 필요하다.

설치기는 관리자 권한의 참석 설치에서 `--install` 또는 `--remove` 하나만 받는다.
이미지와 SHA-256은 설치기에 내장된다. IPC나 받은 manifest가 설치 경로 또는
실행 인자를 지정할 수 없다. 시스템의 Program Files 아래 고정 `Krasis\Input` 위치에
agent, input service, `INPUT-INSTALL.json`을 놓고 `KrasisInput` 서비스를 관리한다.
이 설치기는 전체 앱의 설치·제거 프로그램이 아니다.

설치 transaction은 stage, stop, replace, start, commit과 이전 설치 복원을 구분한다.
중간 checkpoint와 최종 receipt에는 단계·제한된 Win32 오류·복원 결과가 포함된다.
복원 여부가 불확실하면 `recovery_required`이며 설치 성공으로 바꾸지 않는다.
별도 `--repair` 명령이나 사용자 데이터 삭제 옵션은 없다.

일반 권한 UI는 현재 배포본과 설치된 agent의 이미지가 일치할 때 보호 위치의 agent를
선택한다. agent와 SYSTEM 서비스는 고정 이미지·SCM 설정·실제 pipe process·
console session 및 승인된 mTLS peer의 새 lease를 검증한다. 서비스 Running만으로
원격 입력 권한을 부여하지 않는다. ACK 소실 시 입력 재전송이나 backend 전환은 금지한다.

개발용 peer 업데이트는 설치기를 전달하지만 자동으로 관리자 설치를 실행하지 않는다.
보호 이미지 교체에는 받는 PC에서 Windows의 관리자 권한 동의가 필요하다.
SHA-256 일치는 배포자 코드 서명과 다른 보장이다. 현재 개발 EXE를 서명된 정식 배포물로
표시하지 않는다.

## 확인된 범위

업데이트 후 PIN 제어가 끊기면 같은 배포본의 보호 입력 설치를 먼저 확인한다.
g138은 연결 중 선택된 일반 입력 경로를 화면에 표시한다. B의 한 번 설치와
실기기 재검증 지시문은 [g138 복구 작업](../experiments/requests/2026-10-03-g138-pin-recovery.md)에 있다.
앱 업데이트 완료, 설치 성공, 보호 경로 선택, 실제 PIN 제어 성공은 각각 확인한다.

HP15↔GRAM13 g116~g118 시험의 사용자 보고에서 설치 성공, 보호 agent·서비스 일치,
일반 입력, A의 Esc와 마우스 취소 클릭으로 B PIN 창 닫힘이 확인됐다.
PIN 값 입력은 시험하지 않았다. 전환 지연은 별도 조사 중이며, 이 관측을 모든 Windows
기기·보안 화면·설치 실패 복구의 검증으로 확대하지 않는다.

g138에서도 양쪽 보호 입력 설치 후 A의 Esc·마우스 취소 클릭으로 B PIN 창 닫힘이
다시 확인됐다. [B 시험 결과](../experiments/results/2026-10-03-g138-b-pin-recovery.md)에
현장 보고와 수신 진단 대조를 구분해 기록했다. 버벅임은 아직 해결되지 않았다.

## #16에 남은 작업

- 배포자 Authenticode 서명·검증과 재현 가능한 서명 후보, SBOM, checksum 출처
- 전체 앱의 설치·업그레이드·복구·제거·시작 등록 정책과 사용자별/기기별 경계
- Krasis가 소유한 최소 방화벽 설정의 표시·되돌리기·제거
- 설정, identity·trust, 진단·로그, cache를 나눈 선택적 삭제
- 지원 Windows·실기기에서 설치 중단, 복원, 제거, 버전 교체 및 안전 입력 해제 확인

코드와 로컬 테스트, 사용자가 확인한 두 기기 결과, 외부 배포 승인은 구분한다.
#16은 남은 출시 조건을 확인하기 전까지 열어 둔다.
