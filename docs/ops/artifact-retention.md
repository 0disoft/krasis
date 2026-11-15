# 개발 산출물 보관

Windows 빌드가 성공하면 `scripts/prune-artifacts.mjs`로 오래된 프로그램 산출물을 정리한다.
앱 설치·업데이트·연결·서비스 조작이나 데이터 삭제 작업을 대신하지 않는다.

## 대상과 보관 기간

| 경로 | 삭제 가능한 산출물 | 기본 보존 |
| --- | --- | --- |
| `dist/workspace/g<세대>` | 구성 목록과 모든 SHA-256이 맞는 평면 배포 묶음 | 생성·수정 후 48시간, 정상 최신 2개 |
| `dist/app-setup/g<세대>` | 메타데이터·SHA-256이 맞는 setup EXE와 생성 리소스 | 생성·수정 후 48시간, 정상 최신 2개 |
| `dist/input-installer/<revision>` | 메타데이터·SHA-256이 맞는 보호 입력 설치기와 생성 리소스 | 생성·수정 후 48시간, 정상 최신 2개 |

디렉터리와 파일의 생성·수정 시각이 모두 48시간을 넘어야 삭제할 수 있다.
새로 복사한 파일과 최근 수정된 파일이 하나라도 있으면 묶음 전체를 남긴다.
정상 최신 2개는 무결성 검사를 통과한 빌드 중에서 고른다. 손상되거나 불완전한 새 빌드는
기존 정상 복구본을 밀어내지 않는다.

다음 보존 조건은 나이 제한보다 우선한다.

- 사용자가 지정한 `g528`과 기존 복구 기준인 `g535`, `g536`.
- 소스의 현재 `AgentGeneration`, 실행 중인 프로그램과 명령에서 참조하는 묶음.
- 실제 설치의 `launch/current.json`, `previous.json`, `recovery-task.json`에 기록된 세대와 revision.
- Program Files에 설치된 보호 입력 agent/input의 해시와 일치하는 원본 설치기와 배포 revision.
- 보존하는 전체 설치본의 원본 배포 묶음, 보존하는 배포 묶음과 검토용 빌드가 참조하는 입력 설치기.
- 호출자가 명시한 경로와 `dist/artifact-retention-pins.json`의 추가 핀.

설치된 앱, 시작·복구 등록, identity·trust·control·설정·진단·clipboard 데이터,
가상 디스크, 검토 증빙과 알 수 없는 파일은 이 정책의 삭제 대상이 아니다.
`dist/review`의 빌드 기록은 의존성 보호를 위해 읽지만 해당 폴더를 삭제하지 않는다.

## 자동 실행

`build-workspace-update.mjs`, `build-app-setup.mjs`, `build-input-installer.mjs`가
검증까지 성공한 뒤 호출한다. `build-workspace.ps1`도 업데이트 빌더를 통해 같은 정리를 사용한다.
중첩 호출과 반복 빌드는 정책 해시가 같은 성공 기록을 한 시간 동안 재사용한다.
다른 빌드·설치기가 활동 중이면 정리를 보류한다.

실행·복구 기록을 읽을 수 없거나 링크·잠금·알 수 없는 메타데이터를 만나면 삭제를 보류한다.
추가 데이터나 알 수 없는 파일이 섞인 묶음은 그대로 남긴다. 삭제 전에는 실행 중 참조와
상태·파일 해시를 다시 검사하고, 기록이 달라지면 중단한다. 알려진 파일만 개별 삭제하고
비어 있는 묶음 폴더를 제거한다. 재귀 삭제와 설치 소유권 receipt 변경은 하지 않는다.
정리 실패는 경고이며, 성공한 빌드의 종료 코드나 표준 출력을 바꾸지 않는다.

동시 실행 잠금과 마지막 집계는 무시되는 `.cache/artifact-retention.lock`,
`.cache/artifact-retention.json`에 둔다. 죽은 잠금 소유자는 PID 생존 여부를 확인한 뒤에만 회수한다.
별도 예약 작업이나 상주 서비스를 설치하지 않는다.

## 수동 실행과 추가 보존

미리보기: `node scripts/prune-artifacts.mjs`.
적용: `node scripts/prune-artifacts.mjs --apply --force`.
`--force`는 시간당 실행 제한만 건너뛰며 보호 규칙은 유지한다.
이번 실행에서 사용할 입력은 `--keep=dist/workspace/g544`처럼 추가할 수 있다.

추가 핀은 로컬 `dist/artifact-retention-pins.json`에 아래처럼 저장한다.
`generations`, `revisions`, `paths`는 선택 항목이며 revision은 전체 40자리 값이다.

```json
{"schema":"krasis.artifact-retention-pins/v1","generations":[528],"revisions":[],"paths":[]}
```

합성 파일 검증: `node --test scripts/prune-artifacts.test.mjs`.
실제 앱이나 입력 서비스를 실행·중지하거나 사용자 데이터를 바꾸지 않고 기간, 기본 핀,
의존성, 최신 정상본, 손상·링크·동시 실행·변경된 파일·복구 기록의 삭제 거부를 검증한다.

되돌리기는 빌더의 자동 호출을 제거하면 된다. 이미 만료 정리한 프로그램은 해당 소스로 다시 빌드한다.
이 정책은 보호한 복구본과 사용자 데이터를 포함한 드라이브 전체의 절대 용량 상한이 아니다.
