# g271 잠금 시 보호 입력 연결 유지

## 이번 증거와 변경

g270 양쪽 진단 요청 `490acaaf631036e5da5a92d09b620e32`에서 B는
2026-10-11 19:09:21 UTC에 `lifecycle_session_lock`, 19:09:22에
`process_stopped`를 기록했다. 다음 `process_started`/`CONNECTED`는
19:10:10, `installed_input_lease_ready`는 19:10:18이다. 정확한 사용자
조작 시각과 PIN 화면 도달 여부는 미확인이며, 이 기록으로 영구 실패를
판정하지 않는다. 잠금 뒤 약 48초간 프로세스가 없었던 사실은 확인된다.

g271은 검증된 보호 입력을 사용하는 활성 연결에서 프로세스를 유지한다.
기존 runner의 hook·입력·lease·transport 정리를 기다린 뒤 현재 데스크톱의
연결을 다시 준비한다. secure desktop에서는 기존 g270 receive-only 경로를
사용한다. 실제 잠금/PIN 시험 성공 여부와 전환 지연은 별도 확인한다.
정리 실패는 자동 재시작하지 않는다. 로그인 전 서비스 승인 정책은 유지한다.

## 이미 적용한 상태

revision `9d78c00b347fedf8740dd115e5ff72b46472440d`, generation `271`.
A doctor: `healthy`, `CONNECTED / B`, 보호 입력 `CONFIGURED`, UI·agent·
설치 agent 버전 `MATCHED`. B 업데이트 요청 `ab95e01963a312c1be8d786e8a204f3e`는
`UPDATED`. 적용 후 양쪽 진단 요청 `56fb1dafd9a0885ffd4493851b39cb03`은
`RECEIVED`; B 설치 receipt는 20:04:09 UTC, result `0`, stage `complete`.
B 새 agent에 `installed_input_selected`, `CONNECTED`, UI에
`ui_takeover_completed`, `launch_record_ready`가 기록됐다.

독립 배포 SHA-256:

| 파일 | SHA-256 |
| --- | --- |
| krasis.exe | 8d01b178188bd41f10a1748f860d94e4c95c17e307c3c4ee3d377c48522a0897 |
| krasis-agent.exe | 6d50033f08fe0cff69e0392db72c7d63fd7b4f35f13521c4f9667f0cc0c65f45 |
| krasis-cli.exe | 0b1dd102762fef2a8da952a1ea19d58a534fcdc3d0858c41c46e021a1862de6d |
| krasis-install.exe | 7af5559de07447310a33f5e36e1e4972c34211f6ac5f206990c96fcc27f53684 |

## B Codex 지시문

Computer Use·UI 도구를 호출하지 않고 읽기 전용으로 진행해 주세요.
자동 적용된 g271을 재설치하거나 로그인 전 승인을 반복하지 마세요.
실행 중 UI 폴더의 BUILD.json과 위 네 독립 해시를 확인하고 같은 폴더 CLI로
`doctor --json`, `status --json`을 확인합니다. `healthy`·버전 MATCHED·
`PROTECTED / CONFIGURED`·A와 CONNECTED가 아니면 시험을 시작하지 않고
현재 상태를 보고합니다. 보안 설정·서비스·transaction은 변경하지 마세요.

제로디가 B를 직접 한 번 잠그고, A 입력으로 잠금 커튼에서 PIN 화면을
열게 합니다. 잠금 시각, PIN 화면 도달 여부, A→B 마우스 이동·클릭·Esc,
전환 중 재연결 유무와 대략적인 시간을 보고받아 구분해 기록해 주세요.
PIN 값을 입력하거나 기록하지 않습니다. 실패 시 B 장치로 로그인해
복구한 여부와 시각을 기록합니다. 재부팅 시험은 아직 하지 마세요.

시험 후 A Codex에 완료를 알리면 A CLI의 `diagnostics collect B --json`으로
양쪽 진단을 함께 받습니다. B에서 수동 내보내기를 반복할 필요가 없습니다.
PIN 전환을 반복해서 강제로 재현하지 말고 한 번의 결과를 먼저 대조합니다.

## 확인할 기록

같은 B 프로세스 run에서 `lifecycle_session_lock` 또는 `lifecycle_desktop_switch`
뒤 `protected_input_reconnecting`, `installed_source_receive_only`, `CONNECTED`,
입력 후 `installed_input_lease_ready`가 이어지는지 확인합니다. 이때
`process_stopped`가 없어야 합니다. Default 복귀 후 다시 연결/capture를
준비하는지도 확인합니다. 정리 실패 또는 FAULTED가 있으면 그대로 보존하며
성공으로 처리하지 않습니다. receive-only에서 B 로컬 장치로 A를 조작하는
경로는 지원하지 않으며, 이번 A→B 수신 시험과 구분합니다.

검증: agentlifecycle·residentagent·productlog 테스트/vet 통과, UI 79개 테스트,
native frontend 빌드, 업데이트 archive 전송 테스트·묶음 검증 통과.
실제 잠금/PIN 조작과 재부팅 시험은 미실시. 변경된 설계 기준은 ADR 0069.
