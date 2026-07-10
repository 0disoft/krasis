# CI

- 상태: Experimental
- provider: GitHub Actions
- workflow: `.github/workflows/ci.yml`

## 실행 경계

`main` push와 모든 pull request에서 Windows job 하나를 실행한다. job은 repository contents read 권한만 가지며 network service, secret, release와 deployment를 사용하지 않는다.

순서는 format 검사, `go test ./...`, `go vet`, Windows CLI build다. Win32 callback의 `LPARAM` 변환 package만 `unsafeptr` 검사를 제외하고 나머지 vet analyzer는 유지한다. 성공한 build는 commit SHA가 포함된 이름으로 `krasis-lab.exe` artifact를 올린다.

checkout, Go setup과 artifact upload action은 tag가 아니라 검토한 commit SHA로 고정한다. Go version은 `go.mod`의 language floor를 사용하고 dependency가 없는 현재 module에서는 setup action cache를 끈다.

## CI가 증명하지 않는 것

hosted runner는 실제 사용자의 interactive desktop 입력 장치와 네 대상 노트북을 대표하지 않는다. 따라서 `diagnose`, low-level hook `capture`, `SendInput` selftest와 `probe`는 CI에서 실행하지 않는다. 이 동작은 대상 Windows session에서 별도 smoke와 장비별 report로 검증한다.

## 실패 정책

- format, test, vet 또는 build 중 하나라도 실패하면 artifact를 만들지 않는다.
- artifact는 배포물이나 서명된 release가 아니라 동일 commit을 수동 장비 실험에 전달하기 위한 실험 binary다.
- branch protection의 required check 지정 여부는 repository 설정에서 아직 `UNDECIDED`다.

## 로컬 대응

로컬 검증은 mustflow의 `krasis_go_format`, `krasis_go_test`, `krasis_go_vet`, `krasis_go_build_lab` intent가 소유한다. 실제 입력 smoke는 `krasis_lab_diagnose_smoke`, `krasis_lab_capture_smoke`, `krasis_lab_selftest_inject_smoke`, `krasis_lab_probe_smoke`로 분리한다.

## 남은 위험

GitHub가 workflow를 실제로 수락하고 `windows-latest` image에서 성공하는지는 push 후 첫 run으로 확인해야 한다. hosted image와 action runtime 변경도 commit SHA pin만으로 완전히 제거되지는 않는다.
