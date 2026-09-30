# ADR 0063: Go 입력 헬퍼의 데스크톱과 OS 스레드 소유권

- 상태: Accepted for desktop-ownership primitive; helper transport and installation pending
- 결정일: 2026-09-30
- 관련: ADR 0062, ADR 0036, `internal/application/runtime.go`

## 확인된 기반

GRAM13/Windows 26100.9457의 g104 로컬 시험에서 SYSTEM·System RID 16384·상승·UIAccess·
콘솔 세션 1인 자식이 Default 데스크톱을 171(0xAB) 권한으로 열고 Esc를 전송해 PIN 창을
닫았습니다. g103과의 동작상 변경은 요청 권한에 DESKTOP_JOURNALPLAYBACK 비트 32를
추가한 것입니다. 이 성공은 로컬 Esc 경로에 한정되며 제품 원격 입력 성공은 아닙니다.

Go 제품 구현은 새로운 프로세스와 설치 경로를 먼저 켜기보다 다음 책임을 분리합니다.

1. **데스크톱 소유권:** 전용 goroutine을 Windows OS 스레드에 고정합니다. 같은 스레드에서
   Default 입력 데스크톱을 열고 연결하며, 모든 입력 작업 전 스레드·문맥·활성 데스크톱을
   다시 확인합니다. 종료 시 이전 데스크톱으로 복구하고 소유한 핸들만 닫습니다.
2. **특권 헬퍼:** 별도 서비스가 검증된 콘솔 사용자·세션의 데스크톱 worker를 소유합니다.
   일반 UI와 resident agent는 보통 권한을 유지합니다. 설치·제거·업데이트의 특권 경계와
   로컬 IPC 인증을 구현하기 전에는 worker를 제품에서 시작하지 않습니다.
3. **입력 승인:** 기존 mTLS·peer trust·session/lease/epoch/sequence 승인 이후의 입력만
   헬퍼로 전달합니다. 같은 사용자로 실행되거나 데스크톱 핸들을 가진 사실만으로 원격
   입력을 허용하지 않습니다. 임의 PID·명령·데스크톱·실행 경로를 받는 API는 만들지 않습니다.
4. **복구:** lease 만료, disconnect, service/worker crash, lock·session 변경과 업데이트 시
   키·버튼 해제를 같은 worker에 전달합니다. 결과가 불확실한 입력은 다른 backend에서
   재실행하지 않습니다. 기존 crash guard와 로컬 긴급 제어를 우회하지 않습니다.

## 데스크톱 소유권 모듈

`internal/inputdesktop`은 Windows 데스크톱 핸들과 스레드 수명을 관리하는 내부 모듈입니다.
서비스·토큰 상승·IPC·네트워크·입력 전송을 구현하지 않습니다. `Run`은 전용 goroutine과
`runtime.LockOSThread`로 범위를 만들며, `Scope.Do`는 그 스레드에서만 작업을 허용합니다.
다른 goroutine, 종료된 scope, 취소된 문맥, non-Default/비활성 데스크톱은 작업 전에 거부합니다.

요청 마스크는 검증된 0xAB이며 desktop flag 0, handle inheritance false를 유지합니다.
SwitchDesktop·ACL·보안 정책·journaling hook·프로세스 토큰은 변경하지 않습니다.
SID·PIN·창 텍스트·클립보드·키 내용을 읽거나 기록하지 않습니다.

이전 데스크톱 복구가 실패하면 해당 OS 스레드는 Go pool로 돌려보내지 않습니다.
`ErrCleanup`을 받은 미래 헬퍼는 process를 종료하고 supervisor가 그 소유 process만
정리해야 합니다. 복구 실패를 무시하고 같은 helper에서 입력을 이어가지 않습니다.
scope callback은 bounded/cooperative 작업이어야 합니다. 문맥 취소는 이미 시작한 입력의
미전송을 증명하지 않으며, `Run`은 callback 종료와 cleanup 결과를 확인한 뒤 반환합니다.
callback의 panic 값은 반환하지 않으며 `Goexit`도 정리 후 오류로 반환합니다. callback이
만든 window나 hook은 callback이 반환하기 전에 해제해야 합니다.

## 현재 통합 상태와 검증

데스크톱 소유권 모듈은 현재 제품 agent와 연결되지 않습니다. native binding의 입력 없는
검사와 가짜 binding으로 thread ownership·취소·panic·cleanup 실패를 검증합니다.
2026-09-30 HP15의 집중 검사에서 10개 테스트가 모두 통과했습니다. 실제 Windows
데스크톱 열기·연결·복구 검사도 생략 없이 통과했으며, checkptr 검사와 `go vet`가
통과했습니다. 이 검사는 입력을 전송하지 않았고 SYSTEM/UIAccess 경로는 시험하지
않았습니다.
제품 버전은 이 내부 기반만으로 승격하지 않습니다. 자동 업데이트도 새 특권 service를
설치하거나 PIN 보호를 해제하지 않습니다.

다음 단계는 bounded/authenticated helper IPC와 서비스 설치·제거·업데이트를 구현한 뒤
제품 입력 adapter를 연결하는 것입니다. 보호창 guard는 헬퍼가 승인·준비된 정확한 세션에서만
대체할 수 있습니다. A→B PIN 입력, 관리자 PowerShell의 mouse/keyboard/paste, 눌린 입력
해제와 서비스 장애 복구는 그 이후 두 기기에서 확인합니다.

## 참조

- [Go LockOSThread](https://pkg.go.dev/runtime#LockOSThread)
- [SetThreadDesktop](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setthreaddesktop)
- [OpenInputDesktop](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-openinputdesktop)
