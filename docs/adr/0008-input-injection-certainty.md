# ADR 0008: 입력 주입 실패는 적용 확실성으로 분류한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows destination input adapter와 Phase 0 crash guard

## 맥락

Application과 crash guard는 key-down 또는 button-down 전에 token을 기록한다. 모든 adapter
오류를 같은 뜻으로 취급하면 `SendInput` 호출 전에 확실히 실패한 입력도 눌린 것으로 남아
불필요한 up을 만들고, 반대로 부분 주입이나 결과 불명 오류를 rollback하면 실제로 눌린 입력을
복구할 owner를 잃는다. 자동 반복 key-down은 이미 기록된 token을 다시 준비하므로 실패한 반복
한 건을 취소하다가 원래 down의 guard 기록을 지우는 문제도 있다.

## 결정

- 입력 adapter 오류는 성공과 별도로 `NOT_APPLIED`, `PARTIALLY_APPLIED`,
  `OUTCOME_UNKNOWN` 확실성을 제공한다.
- `SendInput` 반환값이 `0`이면 요청한 event가 하나도 삽입되지 않은 `NOT_APPLIED`, 요청 수보다
  작지만 `0`보다 크면 `PARTIALLY_APPLIED`, 요청 수를 넘는 불가능한 값은
  `OUTCOME_UNKNOWN`으로 분류한다. 호출 전 context 취소도 `NOT_APPLIED`다.
- 새 press journal 항목은 `NOT_APPLIED`에서만 rollback한다. 기존 token의 반복 press,
  `PARTIALLY_APPLIED`, `OUTCOME_UNKNOWN`과 분류되지 않은 adapter 오류는 보수적으로 유지한다.
- crash guard protocol v2의 `PREPARE_PRESS` ACK는 새 token 생성과 이미 준비된 token을 구분한다.
  새로 준비한 press가 `NOT_APPLIED`일 때만 `CANCEL_PRESS`로 guard 항목을 제거한다.
- release 실패는 확실성에 관계없이 guard와 Application journal을 유지한다. 성공한 up만
  `RELEASED`로 제거한다.
- 오류와 evidence에는 확실성 및 aggregate event 수만 허용한다. token code나 실제 입력 내용은
  로그 또는 report에 추가하지 않는다.

## 결과

호출 전 취소나 `SendInput` zero result는 유령 press 기록을 남기지 않는다. 부분 주입과 불가능한
반환값은 cleanup owner를 유지한다. 반복 down 실패는 기존 held token의 guard 기록을 지우지 않는다.

이 분류는 `SendInput` 반환 계약을 이용한 software certainty다. 전원 차단, guard와 agent의 동시
종료, Windows 내부에서 반환 뒤 발생하는 문제와 실제 장비별 UIPI 동작까지 증명하지 않는다.

## g207: bounded injection and independent cleanup

The serialized runtime gives each admitted input action a two-second context,
matching the protected worker's I/O budget. An earlier session deadline still
wins. Adapters must finish or retire their operation before returning; the
runtime does not detach injection into a goroutine or accept a queued heartbeat
ahead of it. After an expired or canceled injection, key-up recovery gets a fresh
two-second context that retains the original values without its cancellation.
A late successful adapter return still faults the expired operation. Failed
releases retain their ledger and fault state for the existing recovery owner.
No uncertain operation is retried on another backend.

Tests distinguish a stalled adapter from network delivery: a complete input
frame is available before a heartbeat, injection waits until its deadline,
cleanup releases the potentially applied key, and the heartbeat is never
accepted as a liveness receipt. A native call that ignores context cannot be
preempted by this budget; it remains synchronously owned until it returns.
Two-PC input, protected-window reentry and latency still require device checks.

## g208: Admission and release-plan cancellation

The same input budget now starts before the peer write-coordinator gate and
runtime lock. A canceled runtime waiter does not advance the input stream and
still records a rejected disposition. Runtime failure and recovery-state queries
honor the cleanup caller's deadline; an unavailable snapshot is not guessed as
LOCAL. Service-client and protected-lease waiters likewise cancel before sending
input, with elapsed lock waits retained in the existing duration-only profile.

ReleaseAll stops submitting the remaining plan after context expiry. Its report
keeps the existing plan-based Attempted count and conservation with Released and
Remaining. Every unsubmitted or unconfirmed token stays in the pressed ledger.
Ordinary release failures still allow other tokens to be released within the
budget. Tests cover a slow key-up followed by fresh recovery, failed cleanup
followed by a later successful recovery, and cancellation between two releases.
These changes do not preempt a native call that ignores its context or authorize
another backend to replay uncertain input.

## 대안

- 모든 실패에서 journal 유지: stuck input에는 안전하지만 확실한 무주입에도 불필요한 up을 만든다.
- 모든 실패에서 rollback: 부분 주입과 결과 불명에서 실제 held input의 복구 owner를 잃어 기각한다.
- 반복 press를 별도 token으로 저장: wire와 cleanup 상태를 불필요하게 키우고 Windows의 held 상태와
  맞지 않아 채택하지 않는다.
