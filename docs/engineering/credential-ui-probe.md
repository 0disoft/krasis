# PIN 창 입력 시험

이 시험은 B 노트북의 Windows 보안 PIN 창이 관리자 권한의 입력을 받는지 확인합니다. **g59가 PIN 창에서 연결을 유지하도록 고친 것은 아닙니다.** 시험 중에도 기존 연결은 끊길 수 있습니다.

1. A와 B의 Krasis 제목에 `g59`가 표시되는지 확인합니다. B가 구버전이면 A의 **기기 업데이트**에서 B로 g59를 보냅니다. 파일을 직접 복사할 필요는 없습니다. A가 아직 구버전이면 기존 관리 앱을 트레이에서 종료하되 에이전트는 유지하고, `dist/workspace/g59/krasis.exe`를 실행합니다.
2. B에서 PIN 창을 닫은 상태로 **PowerShell**을 엽니다. 아래 명령 세 줄을 붙여 넣고 Enter를 누릅니다.

   ```powershell
   $agentPath = Get-CimInstance Win32_Process -Filter "Name='krasis-agent.exe'" | Select-Object -First 1 -ExpandProperty ExecutablePath
   if (-not $agentPath) { throw 'B에서 Krasis 에이전트를 찾지 못했습니다.' }
   Start-Process -FilePath $agentPath -ArgumentList 'credential-ui-probe' -Verb RunAs
   ```

3. 관리자 권한 확인창에서 **예**를 누릅니다. 60초 안에 앞서 문제가 생긴 Chrome 로그인 페이지에서 Windows 보안 PIN 창을 띄웁니다. **PIN은 입력하지 않습니다.** 시험 프로그램은 PIN 창이 확인되면 Esc를 한 번 눌렀다 떼고, 결과 알림을 표시합니다.
4. 알림의 영문 코드와 PIN 창이 실제로 닫혔는지 알려주세요. `dialog_dismissed_candidate`는 창이 사라졌을 *가능성*만 뜻하므로 눈으로 확인해야 합니다.

| 결과 코드 | 뜻 |
| --- | --- |
| `dialog_dismissed_candidate` | Esc 전송 뒤 PIN 창이 전면에서 사라짐. 실제로 닫혔는지 확인 필요 |
| `dialog_still_open` | Esc 전송 뒤에도 PIN 창이 전면에 있음 |
| `dialog_not_observed` | 60초 안에 대상 PIN 창을 확인하지 못함 |
| `escape_send_failed` | Esc 입력 전송 실패 |
| `not_elevated` | 관리자 권한으로 실행되지 않음 |

이 시험은 별도의 일회성 프로세스로 실행되며 평소 에이전트를 새로 시작하거나 A와 B를 연결하지 않습니다. 창이 닫히더라도 A의 마우스와 키보드가 B의 PIN 창에서 실제로 작동한다는 뜻은 아닙니다. 그 기능은 시험 결과를 확인한 뒤 별도로 구현해야 합니다.
