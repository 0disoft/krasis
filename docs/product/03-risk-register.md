# 위험 등록부

- 상태: Draft
- 책임자: cherr

| 우선순위 | 위험 | 실패 모습 | 먼저 할 검증 | 기본 대응 |
| --- | --- | --- | --- | --- |
| Critical | source 입력이 억제된 채 복구되지 않음 | 사용자가 현재 노트북을 조작할 수 없음 | 프로세스 강제 종료와 adapter 장애 | 준비 확인 전 억제 금지, watchdog, `LOCAL` 복귀 |
| Critical | 원격 기기에 키나 버튼이 눌린 채 남음 | 단축키 오작동, 계속 드래그됨 | 패킷 유실과 전원 차단 | 눌림 journal, `ReleaseAll`, 새 세션 |
| Critical | 합성 입력이 다시 전달됨 | 입력 폭주 또는 무한 루프 | 주입 표식 누락과 hook 재진입 | 합성 입력 식별, fail-open |
| High | 두 기기가 동시에 제어권을 가짐 | 충돌, 순서 역전 | 동시 경계 진입 | 단일 lease와 epoch |
| High | 공용 Wi-Fi가 peer 통신을 차단 | 발견 또는 연결 실패 | 스터디카페 실망 테스트 | 직접 주소 입력, 대체망 운영 안내 검토 |
| High | 높은 polling rate 또는 터치패드에서 hook 지연 | 커서 끊김, 입력 누락 | 실제 장비 부하 측정 | 최소 작업 hook, native fallback 판단 |
| High | 절전과 잠금 뒤 낡은 세션이 살아남음 | 엉뚱한 기기로 입력 전달 | Modern Standby와 잠금 반복 | 세션 폐기 후 명시적 재연결 |
| High | 관리자 앱 또는 보안 화면 기대가 어긋남 | 일부 창에서 입력 불가 | 권한 수준별 테스트 | MVP 제한을 UI와 문서에 명시 |
| Medium | Wails v3 prerelease 변화로 UI 빌드가 깨짐 | 설치물 재현 불가 | exact toolchain 재현 | 호환 세트 고정, UI를 core에서 격리 |
| Medium | 진단 로그가 사용자 입력을 노출 | 개인정보 유출 | 지원 번들 검토 | allowlist 로그와 민감값 redaction |

정량 임계값은 아직 승인하지 않는다. 지연, soak 시간, 재시도 간격, 경계 감도는 Phase 0 측정 결과로 별도 예산에 확정한다.
