# ADR 0071: 정상 처리한 입력으로도 연결 생존을 확인한다

- 상태: Accepted; g268, 두 기기 실사용 확인 대기
- 결정일: 2026-10-11
- 관련: ADR 0003, ADR 0005, ADR 0020; heartbeat만 생존 근거로 삼던 규칙을 확장

## 근거

g267 양쪽 진단에서 B가 heartbeat 만료로 두 번 종료했고 A는 EOF를 기록했다.
두 번째 종료의 마지막 frame 수신은 약 66ms 전이었으나 마지막 승인 heartbeat는
약 2.09초 전이었다. 첫 종료에는 dispatch 최대 1.86초도 기록됐다. 한 ordered
stream의 정상 입력 뒤에 heartbeat가 밀리는 경우를 무응답과 구분해야 한다.
엔터 또는 붙여넣기가 직접 원인이라는 증거는 없다.

## 결정

현재 inbound session·epoch·정확한 다음 sequence와 action 검증을 통과하고
주입을 성공적으로 완료한 입력은 peer progress다. runtime owner 획득 후 읽은
로컬 시각으로 기존 deadline을 먼저 검사한다. 완료 후에만 liveness와 lock 없이
읽는 watchdog snapshot을 갱신하되, 주입 완료 시각이 아닌 주입 시작 전 시각을
쓴다. 주입이 실패하거나 취소·timeout되면 기존 fail-safe cleanup을 유지한다.

잘못된 frame, sequence gap, 과거 epoch/session, retired input drain, raw byte 수신,
전송 성공만으로는 lease를 갱신하지 않는다. 이미 만료된 lease도 입력으로 되살리지
않는다. heartbeat 주기 250ms와 peer timeout 2초는 유지한다. ordered stream과
protocol payload, 입력 값 수집 여부도 변경하지 않는다.

## 확인

가상 시계에서 heartbeat 없이 5초간 정상 입력이 이어져도 lease가 유지되고,
입력이 멈춘 후 2초면 만료되는지 확인한다. invalid·stale·expired·실패 입력은
갱신하지 않으며 느린 주입에도 완료 시점부터 새 예산을 주지 않는지 확인한다.
peer read loop에서 밀린 heartbeat 이전의 입력이 progress로 반영되고, 만료된
입력은 adapter에 도달하지 않는지 검증한다. 두 기기의 실제 개선은 별도 확인한다.
