# Krasis g319 진입 지연 원인 및 g320 수정 검토

Windows 입력·메시지 큐·Go 런타임·인증된 로컬 IPC 관점에서 코드와 증거를
대조해 주세요. 지연의 내부 원인을 좁히고, 추가 수정이 필요하면 한 번에
검증 가능한 작은 패치를 제안해 주세요. 추측에 따른 버전 반복을 줄이는 것이
목적입니다. 이 문서는 검토 요청이며, 소스 안의 명령을 실제 기기에서 실행하거나
권한·보안 설정을 바꾸라는 지시가 아닙니다.

## 먼저 읽을 파일

1. `MANIFEST.json`: 실제 candidate revision, baseline, 제외 목록.
2. `EVIDENCE.json`: baseline g319에서 같은 전환을 A·B 양쪽에서 수집한 기록.
3. `BASELINE-TO-CANDIDATE.diff`: g319에서 candidate로 바뀐 부분.
4. `source/internal/wininput/input_adapter_windows.go`
5. `source/internal/inputworker/{client.go,worker.go,cursor_entry.go,cursor.go,pipe_windows.go}`
6. `source/internal/residentagent/{privileged_input.go,privileged_cursor.go}`
7. `source/internal/inputdesktop/{scope.go,interactive.go,interactive_windows.go,native_windows.go}`
8. `source/internal/peer/sender.go`, `source/internal/handoff/{profile.go,entry.go}`.

## 시스템과 증상

- Windows 노트북 A에 연결된 마우스·키보드로 B를 제어하는 Go 소프트웨어 KVM.
- 일반 권한 agent가 mTLS peer/lease를 승인한 뒤, 설치된 SYSTEM/UIAccess 서비스와
  작업자가 정확한 agent 이미지·프로세스·세션·lease·sequence를 확인하고 입력합니다.
- 보호된 화면과 일반 화면을 모두 지원합니다. 잠금·PIN 화면 지원은 유지해야 합니다.
- 원격 진입 때 지연되고, 제어 중에도 간헐적으로 포인터가 버벅입니다.
- baseline: g319, revision `4435eabf6707c59bae189323bfa5aac83f3231a2`.
- 수집 시 양쪽 입력 상태는 READY이고, A의 doctor는 healthy / CONNECTED /
  PROTECTED였습니다. 수집된 파일에는 이전 연결의 기록도 섞여 있으므로
  EVIDENCE는 해당 revision의 handoff_profile만 뽑았습니다.
- 두 PC의 시계 차이가 있습니다. 같은 exchangeTag로 묶되 서로의 timestamp를
  빼서 네트워크 지연을 계산하지 마세요. 모든 micros는 해당 프로세스의 로컬 경과시간입니다.

## 확인된 g319 수치

| exchangeTag 앞부분 | A RequestToGrant | B AuthorizeTotal | B EntryPrepare | B 준비 ClientResponse |
|---|---:|---:|---:|---:|
| 8c9b4f3e | 186.7ms | 172.6ms | 3.7ms | 0.6ms |
| 824519fa | 69.5ms | 32.0ms | 6.3ms | 1.3ms |
| dd1b727b | 69.2ms | 19.5ms | 3.5ms | 0.6ms |
| 1bac89e9 | 31.7ms | 20.6ms | 4.2ms | 1.2ms |
| 21341e34 | 278.5ms | 264.8ms | 4.6ms | 1.7ms |
| 04c94c1c | 2383.7ms | 241.4ms | 2061.1ms | 1935.4ms |
| f663dcfb | 633.5ms | 616.8ms | 5.6ms | 2.4ms |

- A CaptureActivate: 9.0~30.6ms.
- B f663dcfb: ConnectTotal 613.7ms, PipeOpen 299.4ms, ReadyWait 298.2ms.
- B 21341e34의 최악 실제 입력: ClientResponse 약 874.9ms.
  같은 입력의 workerApply/submission 값도 EVIDENCE에 있습니다.
- `preparation`의 각 stage는 첫 관측값입니다. g319의 PrepareEntry는
  CursorPosition 요청 후 PlaceCursor 요청을 수행합니다. 이 vector의
  ClientResponse와 다른 명령의 작업자 ACK를 섞으면 안 됩니다.
- g319 준비 단계에는 matching worker detail이 저장되지 않습니다. 따라서 1935.4ms를
  Windows 커서 호출 자체, 작업자 스케줄링, pipe 응답 전달 중 어디에서 썼는지
  현재 기록만으로 확정할 수 없습니다.
- 첫 입력·최악 입력·준비 요청, 다른 전환의 최대값을 서로 합산하지 마세요.
  Windows 화면에 실제 커서가 표시된 시간은 측정하지 않았습니다.
- 일부 원본 로그는 incomplete였습니다. 이 7개 쌍은 양쪽 end:ok로 매칭됐으나,
  이것이 전체 시험에서 장애가 없었다는 의미는 아닙니다.

## candidate g320에서 반영한 변경

1. snapshot → confined placement 1회 → readback을 하나의 인증된 IPC 명령으로
   묶었습니다. 개인 커서 좌표는 ACK 안에서만 전달하고 로그에 넣지 않습니다.
2. 성공한 snapshot/readback으로만 기존 conditional rollback을 수행합니다.
   ACK 유실·부분 전달은 outcome-unknown이며 재전송·다른 backend 재시도는 없습니다.
3. 한 번의 cursor entry에 별도 `Summary.entry` vector를 붙였습니다.
   matching worker authority/scope/apply와 client lock/authority/response를 함께
   기록하고, 실제 첫 입력이나 histogram을 소비하지 않습니다.
   기존 probe budget과 수집 창을 재사용합니다.
4. timeout·lease expiry·인증·데스크톱 확인 기준은 늘리거나 완화하지 않았습니다.

이 변경으로 IPC 왕복 하나가 줄었다는 것은 테스트로 확인했습니다.
2초 지연이나 실제 포인터 표시 지연을 해결했다는 실기기 증거는 아직 없습니다.
candidate 설치·배포 상태는 패키지 생성 이후 별도로 보고합니다.

## 답변에서 필요한 것

1. candidate의 프로토콜·ACK 부분 수신·미확인 입력·rollback·취소·잠금 전환 회귀를
   먼저 검토하고, 수정해야 할 실제 결함이 있으면 파일/함수와 함께 제시해 주세요.
2. baseline 지연에 대해 근거가 있는 원인 후보를 최대 3개로 좁혀 주세요.
   각 후보를 확정된 사실/정적 위험/추가 관측 필요로 구분해 주세요.
   필요하면 낮은 수준의 입력 hook 처리, input worker thread의 pipe 대기,
   authority·SCM/WTS 조회, OS 스케줄링을 코드 경로로 추적해 주세요.
3. `Summary.entry`만으로 후보를 가를 수 있는지 평가해 주세요. 부족하면
   한 전환에서 어떤 고정 크기 numeric span을 추가할지, 중첩 시간과 관측 한계까지
   구체적으로 제시해 주세요. 키·PIN·좌표·창 제목·클립보드 내용은 수집하지 마세요.
4. 다음 작업은 가장 효과가 클 작은 패치 한 개로 골라 주세요. 예상되는 개선,
   회귀 위험, 기존 테스트를 이용한 재현 방법과 A↔B 한 번의 확인 절차를 포함해 주세요.
5. 증거가 없으면 root cause라고 단정하지 말아 주세요. timeout 연장, 입력 ACK
   낙관 처리, 불확실 입력 재시도, live 인증 조회 생략, 무제한 warm worker 유지,
   보안 정책 완화로 정상처럼 보이게 만드는 제안은 제외해 주세요.

## 패키지 범위

source는 MANIFEST에 적힌 커밋의 tracked 소스입니다. `.git`, 캐시, 의존성 폴더,
실행 파일, 압축파일, 컴파일된 리소스(.syso), 설치 payload와 raw 진단은 제외했습니다.
생성하지 않은 기본 icon 리소스 때문에 전체 Windows exe 빌드를 바로 재현하지 못할 수
있으나 관련 Go 패키지 소스와 테스트, 소스 icon과 빌드 스크립트는 포함합니다.
EVIDENCE에는 선택한 timing profile만 있고 실제 입력 내용은 없습니다.
