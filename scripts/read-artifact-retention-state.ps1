param([Parameter(Mandatory)][int]$ControllerPID)
$ErrorActionPreference='Stop'
$processes=@(Get-CimInstance Win32_Process -ErrorAction Stop)
$ancestors=[Collections.Generic.HashSet[int]]::new()
$cursor=$ControllerPID
while($cursor -gt 0 -and $ancestors.Add($cursor)) {
  $parent=@($processes|Where-Object ProcessId -eq $cursor|Select-Object -First 1)
  if(-not $parent.Count){break};$cursor=[int]$parent[0].ParentProcessId
}
$paths=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
$busy=$false
foreach($item in $processes) {
  if($item.Name -match '^(?:krasis(?:-(?:agent|cli|install|input))?|KrasisSetup-g[0-9]+)\.exe$') {
    # Input workers are protected service children; distributions never contain
    # krasis-input.exe. Their installed images are pinned separately by SHA-256.
    if(-not $item.ExecutablePath -and $item.Name -ine 'krasis-input.exe'){throw 'Cannot inspect a live Krasis executable'}
    if($item.ExecutablePath){$null=$paths.Add($item.ExecutablePath)}
    if($item.Name -match '^(?:krasis-install|KrasisSetup-g[0-9]+)\.exe$'){$busy=$true}
  }
  if($item.CommandLine) {
    foreach($match in [regex]::Matches($item.CommandLine,'(?i)dist[\/](?:(?:workspace|app-setup)[\/]g[1-9][0-9]*|input-installer[\/][a-f0-9]{40})(?=\W|$)')) {
      $null=$paths.Add($match.Value.Replace('\','/'))
    }
    if(-not $ancestors.Contains([int]$item.ProcessId) -and $item.Name -in @('node.exe','pwsh.exe','powershell.exe','go.exe') -and
      $item.CommandLine -match '(?i)(?:scripts[\/])?build-(?:workspace(?:-update)?|app-setup|input-installer)\.(?:mjs|ps1)(?=\W|$)'){$busy=$true}
  }
}
@{paths=@($paths);busy=$busy}|ConvertTo-Json -Compress
