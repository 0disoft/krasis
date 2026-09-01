# Resident agent operations

- 상태: Active development runbook; unsigned source build only
- executable: `krasis-agent.exe`

## 시작

agent는 elevation 없이 현재 interactive Windows user session에서 실행한다. `LOCALAPPDATA`가 없거나
product root·identity·control ACL이 정책과 다르면 fail-closed한다. 같은 session의 두 번째 instance는
기존 process를 종료하거나 교체하지 않고 `already running` 오류로 끝난다.

현재 source build는 #14 product configuration adapter 전 단계다. status, diagnostics와 safe stop은
동작하지만 connect와 pair는 `not_configured`를 반환한다. 이를 연결 성공으로 취급하지 않는다.

## 정상 종료

Ctrl+C, authenticated safe stop, Windows lock·suspend·resume·logoff·shutdown·display change는 새 local
request를 막고 active shared session을 cancel한다. session은 최대 5초 안에 cleanup과 `LOCAL` 수렴을
마쳐야 한다. timeout은 성공으로 바꾸지 않는다.

종료 뒤 `control\endpoint.json`과 `control\auth-token`이 남아 있으면 stale credential cleanup 실패다.
다음 실행은 새 token을 발급하지만 남은 file을 정상 증거로 간주하지 않는다.

## Crash와 stale owner

process crash 시 Windows가 mutex handle을 회수하므로 다음 실행은 별도 PID file 삭제 없이 owner가 된다.
crash 중 active input recovery는 기존 crash guard와 shared session cleanup 계약이 소유한다. mutex 재획득
자체가 held input release 증거는 아니다.

## Update handoff

unsigned development build에는 updater가 없다. #16 installer는 authenticated safe stop을 요청하고 agent
종료와 control credential 제거를 확인한 뒤 binary를 교체해야 한다. 실행 중 owner mutex를 강제로 빼앗거나
healthy process를 kill해 update하지 않는다.
