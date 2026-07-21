# ADR 0026: Phase 0 버튼 UI는 입력 agent 밖의 native launcher로 둔다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows Phase 0 실행 UX, child lifecycle, local config와 report clipboard
- 관계: ADR 0001의 agent/projection 분리와 ADR 0007의 lifecycle owner를 유지한다.

## 맥락

실제 S-A/S-B 검증은 긴 PowerShell 명령, 역할별 credential 경로와 매번 다른 report 이름을 요구한다. 반복
입력은 잘못된 폴더, 만료된 credential, 같은 report 덮어쓰기와 다른 run의 JSON 복사로 이어졌다. 하지만
편의를 이유로 Wails/Svelte 또는 GUI event loop를 hook·network·lease owner로 올리면 이미 검증한 agent
안전 경계를 다시 흔든다.

## 결정

- Phase 0 kit에 standard library만 사용하는 별도 Windows GUI `krasis-launcher.exe`를 포함한다.
- launcher는 실시간 입력 경로에 참여하지 않고 `krasis-lab.exe agent-session` child를 시작한다. hook,
  suppression, mapping, transport, injection, lease와 crash guard owner는 바뀌지 않는다.
- child는 console 없는 숨김 process로 실행한다. launcher는 `Process.Kill`을 사용하지 않는다. 현재 run과
  같은 report directory의 사전 검증된 `--stop-file`만 생성하고 agent의 context cancellation,
  `ReleaseAll`, guard disarm과 report flush를 기다린다.
- config는 executable 옆 strict JSON `krasis.launcher.config/v1` 하나다. 알려지지 않은 field, 절대 경로,
  parent traversal, 잘못된 role/alias/edge와 1초 미만 duration을 거부한다. private key와 report body는
  config에 넣지 않는다.
- launcher는 config나 credential을 읽기 전에 `SHA256SUMS.txt`의 정확한 package manifest와 lab, launcher,
  helper, runbook, build metadata hash를 검증한다. 불일치하면 child 실행 버튼을 fail-closed로 잠근다.
  runtime-owned config/report/secret directory는 checksum entry가 아니며 외부 origin authenticity는 별도다.
- launcher는 start 시 만든 정확한 agent/guard/stop 경로를 memory에서 묶는다. `보고서 복사`는 directory의
  newest file을 검색하지 않고 이 두 bounded JSON object만 `krasis.launcher.report-bundle/v1`로 합친다.
- Pair 준비는 CLI의 exclusive output, DACL protection과 trust import를 그대로 순차 호출한다. listener는
  connector directory를 Explorer에 보여줄 뿐 자동 전송하거나 bundle 전체를 복제하지 않는다.
- 한 번에 한 child operation만 허용한다. GUI close 중 agent session이 실행 중이면 안전 중지를 요청한 뒤
  child completion 후 창을 닫는다.

## 결과

사용자는 정상 실험에서 PowerShell 명령을 다시 입력하지 않아도 된다. CLI는 여전히 독립적으로 실행 가능하고
launcher 실패 시 수동 fallback이다. GUI가 멈추거나 종료돼도 agent/crash guard 안전 모델을 우회하지 않는다.

report clipboard에는 원시 입력, 좌표, credential과 local path가 추가되지 않는다. 반면 clipboard 자체는
다른 local process가 읽을 수 있으므로 사용자가 외부 AI에 전달한 뒤 민감한 다른 내용을 덮어쓰는 UX는
정식 제품 단계에서 별도 설계한다.

## 대안

- PowerShell shortcut 또는 `.cmd`: 초기 클릭 수는 줄지만 role/path 검증, 정확한 report 추적과 안전한 상태
  표시가 약하고 Windows 전용 persistent script를 늘려 기각한다.
- Wails v3 + Svelte를 즉시 도입: 정식 tray UI 후보지만 Phase 0 launcher에 프리릴리스 runtime과 WebView2
  packaging을 추가하고 검증 범위를 넓혀 보류한다.
- 기존 `krasis-lab.exe`에 GUI를 합치기: console/GUI subsystem과 실시간 owner를 한 binary에 섞어 장애
  격리를 약하게 하므로 기각한다.
- child 강제 종료: guard 복구 실험에는 필요하지만 정상 UI stop으로 사용하면 report flush와 clean shutdown을
  일부러 깨므로 기각한다.

## 검증 gate

- config strict parse, role/path/duration 경계와 atomic replace를 unit test로 검증한다.
- session plan이 unique한 세 output 경로와 정확한 address/credential arguments를 만드는지 검증한다.
- stop-file monitor가 regular signal을 소비해 canceled path로 수렴하고 existing/nonregular path를 거부하는지
  검증한다.
- report bundle이 정확한 두 JSON object만 포함하고 local path·multiple JSON·oversize input을 거부하는지
  검증한다.
- Windows build와 package smoke는 lab·launcher 두 executable을 checksum manifest로 검증한다.
- launcher core test는 배포 file tamper를 차단하고 runtime-owned local data가 재실행을 막지 않는지 검증한다.
- 실제 GUI click, 두 노트북 연결, 안전 중지와 clipboard 내용은 자동 test가 live input을 억제하지 않도록
  사람의 S-A/S-B 실행으로 최종 확인한다.
