# 기기 업데이트

- 상태: 승인한 peer를 통한 개발 빌드 전달·자동 적용 구현
- 기준: `docs/product/02-spec.md`, `internal/peerupdate`, 관리 앱의 update controller

양쪽 기기를 명시적으로 페어링한 뒤, 받는 PC가 상대의 업데이트 수신을 허용해야 한다.
입력 제어 권한과는 별도이며 철회할 수 있다. 송신자는 UI의 ‘업데이트 보내기’ 또는
`krasis-cli update send ALIAS`로 현재 배포 폴더를 전달한다.

관리 채널은 기존 인증서를 pin한 mTLS와 현재 trust를 확인한다. 묶음의 manifest,
generation/revision, SHA-256 및 파일 구성을 검증해 별도 폴더에 저장한다. 관리 앱의
자동 적용은 입력 연결을 안전하게 끝내고 새 UI와 agent를 시작한다. 이전 파일과
페어링 정보는 보존한다. 받는 PC의 관리 앱은 창 또는 트레이에서 실행 중이어야 한다.
에이전트도 종료됐다면 원격 명령을 받을 수 없으므로 해당 PC의 고정 복구 CLI를 사용한다.

작업 공간에서 상대 기기를 고른 뒤 ‘상대 앱 열기’를 누르면 기존 관리 채널로
상대의 마지막 검증된 Krasis 실행 파일을 연다. 입력 연결을 해제한 상태에서도
사용할 수 있다. 상대의 입력 제어 허용을 확인하며 실행 경로나 명령을 전달하지 않는다.
요청은 한 번만 보내고 같은 요청의 응답을 확인한다. 실행 요청 수락과 실제 창 표시를
구분하며, 상대 에이전트도 종료됐다면 응답을 받을 수 없다.

`queued`는 시작, `DELIVERED`는 수신 확인, `UPDATED`는 상대의 정확한 새 agent 빌드
관측이다. 수신과 적용 완료를 동일시하지 않는다. `update wait ALIAS ID`는 작업 ID를
고정해 적용 확인 또는 실패까지 기다린다. 확인이 끊기면 자동으로 다시 전송하지 않는다.

개발 EXE의 checksum 검증은 배포자 코드 서명이 아니다. 공개 배포 경로는 아래의
별도 배포 서명을 확인한다. 실제 외부 배포는 release policy의 승인 조건을 따른다.
g140부터 이미 설치한 보호 입력의 갱신도 새 버전 시작 과정에 포함된다. 해당 PC의
Windows 관리자 확인은 자동 승인하지 않는다. 취소하면 일반 입력으로 앱을 열고,
화면의 보호 입력 갱신 버튼으로 다시 요청할 수 있다. 최초 설치·불확실한 설치 상태를
자동 갱신으로 처리하지 않으며, 설치 결과가 미확인이면 agent 시작을 보류한다.

진단에는 버전·집계 상태·실패 코드만 사용하며 raw 입력, PIN, clipboard 내용과
peer 주소를 기록하지 않는다. 설치·자동 적용의 실기기 증거와 외부 배포 승인은
각각 기록한다.

## 공개 배포 자동 업데이트

앱 아래쪽 ‘자동 업데이트’는 기본으로 켜져 있다. 시작 1분 후와 이후 6시간마다
GitHub의 `releases/latest/download/latest.json`을 확인한다. ‘업데이트 확인’으로
즉시 확인할 수 있다. 옵션을 끄면 주기 확인·다운로드·자동 적용을 멈춘다.
수동 확인은 새 버전의 존재만 확인한다.

피드는 Ed25519로 서명한 base64 payload와 signature다. payload에는 generation,
revision, Windows 플랫폼, ZIP 주소·크기·SHA-256을 담는다. 빌드에 내장한 공개키로
서명을 검증하고, 고정 GitHub 저장소의 새 generation만 받는다. 다운로드 후 기존
updatebundle 검증·별도 폴더 저장·앱/에이전트 교체를 재사용한다.

연결 중에는 다운로드까지만 진행한다. 연결과 페어링을 끝낸 뒤 자동으로 적용한다.
상대 PC가 없는 단독 설치에서도 적용할 수 있다. 인터넷 오류나 잘못된 서명은 현재
앱을 종료하지 않는다. peer 업데이트와 공개 업데이트의 적용은 같은 잠금으로 직렬화한다.

배포 환경에는 `KRASIS_RELEASE_PUBLIC_KEY`(base64 Ed25519 공개키)와
`KRASIS_RELEASE_PRIVATE_KEY`(PKCS8 PEM 서명키)가 필요하다. 서명키는 GitHub release
환경의 Secret, 공개키는 Variable로 등록한다. 실제 키가 없으면 ‘공개 배포 준비 중’으로
표시하고 파일을 실행하지 않는다. 배포 서명은 Windows Authenticode 서명과 별개다.

깨끗한 커밋에서 `KRASIS_RELEASE_PUBLIC_KEY`를 설정하고 `scripts/build-workspace.ps1`로
만든 폴더를 `node scripts/sign-release.mjs dist/workspace/g숫자`에 전달한다.
서명기는 실제 EXE의 공개키 내장 여부·패키지 검증을 확인한 뒤 ZIP과 latest.json을 만든다.
GitHub release 워크플로는 승인된 기존 `g숫자` 태그를 수동 지정할 때만 실행한다.
평상시 push나 개발 빌드에서는 공개 릴리스를 만들지 않는다.
