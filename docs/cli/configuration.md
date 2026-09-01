# CLI configuration

- Status: Accepted
- Owner: product CLI
- ADR: `docs/adr/0037-product-cli-and-local-api-projection.md`

## Product precedence

`krasis.exe`가 local API control directory를 정하는 순서는 다음과 같다.

1. 명령마다 명시한 `--control-dir` absolute path
2. `%LOCALAPPDATA%\Krasis\config.json`의 `controlDirectory`
3. 제품 기본값 `%LOCALAPPDATA%\Krasis\control`
4. 선택된 control directory 안에서 실행 중 agent가 게시한 `endpoint.json`과 `auth-token`

환경 변수, current directory, registry, lab launcher config와 Phase 0 credential directory는 설정 source가
아니다. endpoint address와 token은 user config로 복사하지 않고 agent 재시작마다 runtime state로 다시
읽는다.

## User config schema

```json
{
  "schemaVersion": "krasis.cli-config/v1",
  "controlDirectory": "C:\\Users\\user\\AppData\\Local\\Krasis\\control"
}
```

파일은 64 KiB 이하의 단일 strict JSON object다. unknown field, trailing value, 지원하지 않는 schema와
relative 또는 whitespace-padded control path는 거부한다. 파일이 없으면 제품 기본값을 사용한다.
명령줄 override와 config 값은 token이나 certificate를 포함할 수 없다.

## Agent runtime state

control directory의 endpoint와 bearer token은 agent만 생성·삭제한다. CLI는 보호 상태와 schema를
검증한 뒤 loopback endpoint에만 접속한다. endpoint가 없으면 exit `4`, credential이 손상됐거나 보호
검증에 실패하면 exit `5`다. CLI는 해당 파일을 복구, 덮어쓰기 또는 로그 출력하지 않는다.

## Lab boundary

`krasis-lab`은 문서화된 explicit flag만 사용한다. 제품 config나 local agent API를 읽지 않으며 제품
CLI와 exit-code 호환성을 약속하지 않는다.
