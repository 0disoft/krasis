# 장비별 probe 실행

- 상태: Experimental
- 대상: `H-A`, `H-B`, `S-A`, `S-B`
- 실행 방식: 사용자 수동 실행 전용

## 전제

GitHub Actions의 `krasis-probe-kit-windows-amd64-<commit>.zip` artifact를 한 번 내려받아 두 장소에서 같은 zip을 사용한다. 이 artifact는 서명된 release나 설치 프로그램이 아니다. Windows가 출처 경고를 표시할 수 있으며, commit과 checksum을 확인할 수 없는 복사본은 실행하지 않는다.

네 대를 동시에 켤 필요가 없다. 집에서 `H-A`, `H-B`를 각각 실행하고 다른 날 스터디카페에서 `S-A`, `S-B`를 실행해도 된다. 각 장비에서는 다른 프로그램의 중요한 작업을 저장하고 마우스나 키보드를 자동 조작하는 프로그램을 잠시 끈다.

## 무결성 확인

압축을 푼 directory에서 `SHA256SUMS.txt`에 기록된 값과 `krasis-lab.exe`의 SHA-256을 비교한다. 아래 PowerShell은 사람의 수동 확인용이며 agent command 권한을 부여하지 않는다.

```powershell
$expected = (Get-Content -LiteralPath .\SHA256SUMS.txt -Raw).Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries)[0]
$actual = (Get-FileHash -LiteralPath .\krasis-lab.exe -Algorithm SHA256).Hash.ToLowerInvariant()
$expected -eq $actual
```

결과가 `True`가 아니면 실행하지 않는다.

## 수집

장비 별칭을 실제 장소와 노트북에 맞게 하나만 선택한다. 아래 명령은 사람의 수동 실행용이다.

```powershell
.\collect-probe.ps1 -DeviceAlias H-A
```

기본 `Basic` mode의 capture 구간은 30초다. 안내가 나오면 그 구간에 pointer 이동, wheel, 기본 mouse button과 keyboard를 각각 사용한다. script는 `reports` directory 아래에 UTC timestamp가 포함된 새 JSON을 만들며 기존 보고서를 덮어쓰지 않는다.

입력 종류별 누락을 바로 확인하려면 `Guided` mode를 사용한다. keyboard, pointer, wheel/buttons 세 단계가 각각 기본 10초 동안 실행된다.

```powershell
.\collect-probe.ps1 -DeviceAlias H-A -Mode Guided
```

다른 장비에서는 별칭만 바꾼다. 집은 `H-A`, `H-B`, 스터디카페는 `S-A`, `S-B`다. 같은 노트북에 다른 별칭을 재사용하지 않는다.

`Basic` mode 보고서를 모은 뒤 아래 명령으로 schema와 비교 조건을 확인할 수 있다. 이 명령도 사람의 수동 실행용이다.

```powershell
$reports = @(Get-ChildItem -LiteralPath .\reports -Filter '*-basic-*.json' -File | Select-Object -ExpandProperty FullName)
.\krasis-lab.exe probe-check @reports
```

서로 다른 build revision, dirty build, duration 또는 중복 장비 별칭이 섞이면 비교 불가능으로 판정한다. `Guided` report는 현재 `probe-check` 입력이 아니며 stage별 `passed`와 `failureCodes`를 직접 확인한다.

## 실패 처리

- exit `0`: 모든 단계가 완료된 보고서가 생성됨
- exit `1`: 부분 보고서가 생성됐을 수 있으므로 삭제하지 않고 함께 보관
- exit `2`: 별칭, duration 또는 output 경로 오류
- exit `3`: 현재 platform에서 지원되지 않음

JSON에는 원시 key, 입력 문자열, pointer 좌표, host name과 window title이 없어야 한다. `completed=false`도 실패 분석에 필요한 증거다.

## 아직 증명하지 않는 것

이 probe는 한 장비의 hook, aggregate capture와 tagged `SendInput`만 검사한다. 두 노트북 사이의 연결, 입력 전달, 억제, 절전, 강제 종료와 `ReleaseAll`은 별도 실험이다.
