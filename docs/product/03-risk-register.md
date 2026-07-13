# 위험 등록부

- 상태: Draft
- 책임자: cherr

| 우선순위 | 위험 | 실패 모습 | 먼저 할 검증 | 기본 대응 |
| --- | --- | --- | --- | --- |
| Critical | source 입력이 억제된 채 복구되지 않음 | 사용자가 현재 노트북을 조작할 수 없음 | 프로세스 강제 종료와 adapter 장애 | 준비 확인 전 억제 금지, watchdog, `LOCAL` 복귀 |
| Critical | 원격 기기에 키나 버튼이 눌린 채 남음 | 단축키 오작동, 계속 드래그됨 | 패킷 유실과 전원 차단 | 눌림 journal, `ReleaseAll`, 새 세션 |
| Critical | 합성 입력이 다시 전달됨 | 입력 폭주 또는 무한 루프 | 주입 표식 누락과 hook 재진입 | 합성 입력 식별, fail-open |
| Critical | source boundary observer panic이 hook callback을 이탈함 | 현재 입력이 사라지거나 agent process가 종료됨 | observer panic과 민감한 panic payload 테스트 | 고정 오류로 변환, 현재 event 로컬 통과, source generation 폐기 |
| Critical | source mapper panic이 hook callback을 이탈함 | 현재 입력이 사라지거나 agent process가 종료됨 | mapper panic과 민감한 panic payload 테스트 | remote enqueue 전 고정 오류로 변환, 현재 event 로컬 통과, source generation 폐기 |
| High | wire codec와 runtime의 discrete code domain이 다름 | 인증된 peer의 잘못된 frame이 destination session fault를 유발함 | encoder와 decoder의 invalid key/button code 계약 테스트 | `PressedToken.Validate`를 단일 code-domain source로 사용 |
| High | unauthenticated TLS client가 listener를 독점함 | 정상 peer 연결과 재연결이 무기한 대기함 | partial TLS record와 정상 mTLS 연결의 경합 테스트 | accept/handshake 분리, 5초 timeout, pre-auth 동시성 4개 제한 |
| High | 두 기기가 동시에 제어권을 가짐 | 충돌, 순서 역전 | 동시 경계 진입 | 단일 lease와 epoch |
| High | revoke 뒤 낡은 heartbeat가 기록됨 | 정상 복귀 직후 connection fault와 불필요한 재연결 | heartbeat/revoke 순서 강제 테스트 | ADR 0005 semantic write coordinator 선행 |
| High | timeout 뒤 늦은 heartbeat가 만료 세션을 되살림 | 끊긴 peer가 살아 있다고 판단해 원격 입력 상태가 남음 | deadline 경계의 heartbeat 수신 경합 테스트 | 수신 coordinator 안에서 갱신 전 deadline 판정 |
| High | component 자체 취소를 정상 teardown으로 오인함 | 한 worker만 죽고 agent session과 입력 owner가 남음 | 독립 deadline worker 종료 테스트 | 부모 context가 살아 있으면 전체 session failure로 승격 |
| High | terminal supervisor가 `ACTIVE` snapshot이나 부분 connection을 남김 | tray가 죽은 agent를 활성으로 표시하거나 transport owner가 누수됨 | unsafe/clean return과 partial acquire 테스트 | 모든 terminal return을 `STOPPED`로 수렴하고 부분 connection 회수 |
| High | connection과 listener Close 실패를 삼킴 | 포트나 transport가 남은 채 재시작·retry하고 원래 장애만 보고됨 | 중복 Close와 close-error 결합 테스트 | close-once owner와 원인 보존 오류 결합, 불확실한 close 뒤 retry 금지 |
| High | peer session 직접 호출이 Close를 중복 실행하고 오류를 버림 | EOF나 취소 원인이 가려지거나 cleanup 뒤 transport 상태가 불확실함 | EOF/cancellation과 Close 실패 동시 테스트 | session-local close-once owner와 종료 원인 결합 |
| High | cleanup coordinator timeout이 원래 fault를 가림 | active runtime이 남았는데 timeout만 보여 최초 장애를 진단하지 못함 | coordinator 점유 중 bounded cleanup timeout 테스트 | runtime 무변경, 원래 cause와 deadline 오류 결합, reconnect 중단 |
| Critical | 부분 `ReleaseAll` 실패 뒤 재시도 가능한 token 상태를 버림 | 원격 기기에 일부 키나 버튼이 계속 눌린 채 남음 | transient/지속 release 실패 recovery 테스트 | 같은 coordinator에서 fault recovery 한 번, 성공 token 제외, 지속 실패는 `FAULT` 유지 |
| Critical | network write가 source sender를 막아 fault cleanup도 멈춤 | suppression은 꺼졌지만 destination held input이 남음 | blocked send 중 queue overflow 테스트 | sender/lifecycle 분리, connection 선제 close, bounded cleanup |
| High | 공용 Wi-Fi가 peer 통신을 차단 | 발견 또는 연결 실패 | 스터디카페 실망 테스트 | 직접 주소 입력, 대체망 운영 안내 검토 |
| High | 높은 polling rate 또는 터치패드에서 hook 지연 | 커서 끊김, 입력 누락 | 실제 장비 부하 측정 | 최소 작업 hook, native fallback 판단 |
| High | 절전과 잠금 뒤 낡은 세션이 살아남음 | 엉뚱한 기기로 입력 전달 | Modern Standby와 잠금 반복 | 세션 폐기 후 명시적 재연결 |
| High | 관리자 앱 또는 보안 화면 기대가 어긋남 | 일부 창에서 입력 불가 | 권한 수준별 테스트 | MVP 제한을 UI와 문서에 명시 |
| Medium | Wails v3 prerelease 변화로 UI 빌드가 깨짐 | 설치물 재현 불가 | exact toolchain 재현 | 호환 세트 고정, UI를 core에서 격리 |
| Medium | 진단 로그가 사용자 입력을 노출 | 개인정보 유출 | 지원 번들 검토 | allowlist 로그와 민감값 redaction |
| High | 제품 trust store 경로와 pairing UI가 없어 Phase 0 store를 수동 관리함 | 잘못된 store directory나 alias를 선택하고 key rotation을 놓침 | import/list/revoke, 손상·중복·lock·DACL과 향후 migration 테스트 | atomic `krasis.trust-store/v1`과 immutable runtime snapshot은 구현됨; 제품 default 경로·사용자 승인·rotation UX는 Phase 2 후속 |
| High | press 주입 결과 확실성 분류가 실제 Windows 동작과 어긋남 | 유령 token 또는 실제 held input 누락 | zero·partial·불가능 count, 반복 press, guard cancel과 실제 장비 failure-injection 테스트 | `NOT_APPLIED`인 새 기록만 rollback하고 partial·unknown·기존 반복 token은 guard와 journal에 유지; protocol v2 구현됨 |
| Medium | lease grant 실패 뒤 destination cursor 위치가 복원되지 않음 | 제어권은 없는데 destination cursor만 화면 끝으로 이동함 | entry 성공 뒤 runtime/write 실패 테스트 | cursor snapshot/rollback 또는 ready/commit handshake 설계 |
| Medium | 복사된 Windows private key가 넓은 ACL이나 reparse path에 놓임 | 다른 로컬 계정이 key를 읽거나 교체함 | inherited DACL·non-regular key·역할별 복사 후 재보호 테스트 | 생성·명시적 재보호 시 현재 사용자와 `SYSTEM` 전용 protected DACL 강제, load-time DACL·reparse 검증 구현됨; 이미 노출된 key는 폐기 |
| High | Windows lifecycle message가 누락되거나 monitor cleanup이 멈춤 | stale lease나 geometry가 resume 뒤 남음 | OS message fault-injection, Modern Standby와 실제 장비 반복 | 숨겨진 window owner가 잠금·절전·복귀·로그오프·shutdown·display 변경을 terminal cancel하고 source·guard·monitor를 close-once 종료함; 다음 실행에서만 재진단·rearm |

정량 임계값은 아직 승인하지 않는다. 지연, soak 시간, 재시도 간격, 경계 감도는 Phase 0 측정 결과로 별도 예산에 확정한다.
