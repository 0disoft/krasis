Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$snapshot = 'f709673a9f56de3ca567b0ce52ca1f4b67eb0e21'
$deployed = '9d78c00b347fedf8740dd115e5ff72b46472440d'
$request = '435aaed955d72e59da0ec10d1b199397'
$peer = '1aeff94249a3a7a76cfa2765948a731e4b57cc898da1ab79f1bd64fa2583e3f0'
$questionPath = Join-Path (Get-Location) 'docs/reviews/g271-lock-pin-question.md'
$question = [IO.File]::ReadAllText($questionPath)
if (-not $question.Contains($deployed)) { throw 'Question deployment revision mismatch' }
$paths = @(& git -c core.quotepath=false ls-tree -r --name-only $snapshot)
if ($LASTEXITCODE -ne 0 -or $paths.Count -eq 0) { throw 'Missing source snapshot' }
foreach ($path in $paths) {
    if ($path -match '(^|/)(\.git|\.cache|cache|node_modules|dist|build|target|vendor|diagnostics|__pycache__)(/|$)' -or
        $path -match '(?i)(\.(exe|dll|msix|msi|zip|pem|key|pfx|p12|jsonl|log)$|(^|/)\.env($|\.))' -or
        $path -match '(^/|(^|/)\.\.(/|$)|\\)') { throw "Excluded source path: $path" }
}
$output = Join-Path (Get-Location) 'dist/reviews/g271-lock-pin'
$download = Join-Path $env:USERPROFILE 'Downloads'
$archiveName = 'krasis-g271-lock-pin-review.zip'
$questionName = 'krasis-g271-lock-pin-QUESTION.md'
if ((Test-Path -LiteralPath $output) -or (Test-Path -LiteralPath (Join-Path $download $archiveName)) -or
    (Test-Path -LiteralPath (Join-Path $download $questionName))) { throw 'Review output exists; refusing to overwrite' }
New-Item -ItemType Directory -Path $output | Out-Null
$zipPath = Join-Path $output $archiveName
& git archive --format=zip --prefix=source/ "--output=$zipPath" $snapshot
if ($LASTEXITCODE -ne 0) { throw 'Source archive failed' }

# Only already-sanitized support exports enter the bundle. Keep deployed-build
# events in the bounded trial window, retaining correlation/timing fields and
# incomplete flags. No raw log, profile, credential, process command or PIN copy.
$extras = [ordered]@{ 'QUESTION.md' = $question }
$timeline = [Collections.Generic.List[object]]::new()
$counts = [ordered]@{}
foreach ($side in @('A', 'B')) {
    $suffix = if ($side -eq 'A') { 'local' } else { "peer-$peer" }
    $file = Join-Path $env:LOCALAPPDATA "Krasis/diagnostics/support-request-$request-$suffix.json"
    $raw = [IO.File]::ReadAllText($file)
    $data = $raw | ConvertFrom-Json -DateKind String
    if ($data.schema -ne 'krasis.support/v1') { throw 'Unexpected support schema' }
    $logs = @()
    $count = 0
    foreach ($log in $data.logs) {
        $events = @($log.events | Where-Object {
            ($_.PSObject.Properties.Name -contains 'build') -and $_.build -eq $deployed -and [DateTimeOffset]::Parse($_.time) -ge [DateTimeOffset]::Parse('2026-10-12T06:10:00Z') -and
            [DateTimeOffset]::Parse($_.time) -le [DateTimeOffset]::Parse('2026-10-12T06:26:45Z')
        })
        if ($events.Count -eq 0) { continue }
        $logs += [ordered]@{ source = $log.source; incomplete = $log.incomplete; events = $events }
        $count += $events.Count
        foreach ($event in $events) {
            if ($event.code -match '^(process_|agent_ready|lifecycle_|installed_source_receive_only|protected_input_reconnecting|installed_input_(selected|lease_ready|connect_failed|heartbeat_failed|stage_|native_)|session_|fault_|reconnect_|CONNECTED$|CONNECTING$|RECONNECTING$|DISCONNECTING$)') {
                $timeline.Add([ordered]@{ side = $side; log = $log.source; event = $event })
            }
        }
    }
    if ($count -eq 0) { throw "Missing g271 evidence for $side" }
    $counts[$side] = $count
    $trimmed = [ordered]@{
        schema = $data.schema; created = $data.created; scope = $data.scope
        evidenceNote = 'Sanitized support filtered to deployed g271 events in the trial window. Historical receipts retain their original times.'
        originalSupportSha256 = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
        installation = $data.installation; loginRuntime = $data.loginRuntime; logs = $logs
    }
    $extras["evidence/$side.json"] = $trimmed | ConvertTo-Json -Depth 40
}
$extras['evidence/timeline.json'] = @($timeline | Sort-Object { [DateTimeOffset]::Parse($_.event.time) }, side) | ConvertTo-Json -Depth 40
$manifest = [ordered]@{
    generation = 271; deployedRevision = $deployed; sourceSnapshot = $snapshot
    sourceTree = (& git rev-parse "$snapshot^{tree}").Trim(); sourceFileCount = $paths.Count
    diagnosticRequest = $request; evidenceStartUTC = '2026-10-12T06:10:00Z'; evidenceEndUTC = '2026-10-12T06:26:45Z'
    evidenceEventCounts = $counts
    sourceNote = 'Snapshot adds only the g271 handoff documentation to deployed revision; code matches the deployed source.'
    excluded = @('Git internals', 'build outputs', 'caches', 'installed dependencies', 'runtime configuration', 'credentials', 'raw diagnostics', 'previous-build events')
    reviewEntrySha256 = [ordered]@{}
}
if ($LASTEXITCODE -ne 0) { throw 'Source tree lookup failed' }
foreach ($name in $extras.Keys) {
    $manifest.reviewEntrySha256[$name] = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($extras[$name]))).ToLowerInvariant()
}
$extras['MANIFEST.json'] = $manifest | ConvertTo-Json -Depth 8
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::Open($zipPath, [IO.Compression.ZipArchiveMode]::Update)
try {
    foreach ($name in $extras.Keys) {
        $entry = $archive.CreateEntry($name, [IO.Compression.CompressionLevel]::Optimal)
        $stream = $entry.Open()
        try { $bytes = [Text.Encoding]::UTF8.GetBytes($extras[$name]); $stream.Write($bytes, 0, $bytes.Length) }
        finally { $stream.Dispose() }
    }
} finally { $archive.Dispose() }
$archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
try {
    $expected = @($paths | ForEach-Object { "source/$_" }) + @($extras.Keys)
    $actual = @($archive.Entries | Where-Object { -not $_.FullName.EndsWith('/') } | ForEach-Object { $_.FullName })
    if (@(Compare-Object ($expected | Sort-Object) ($actual | Sort-Object)).Count -ne 0 -or $actual.Count -ne @($actual | Select-Object -Unique).Count) { throw 'Archive inventory mismatch' }
    foreach ($entry in $archive.Entries) {
        if ($entry.FullName.EndsWith('/')) { continue }
        $inputStream = $entry.Open()
        $decoded = [IO.MemoryStream]::new()
        try {
            $inputStream.CopyTo($decoded)
            if ($decoded.Length -ne $entry.Length) { throw 'Truncated entry' }
            if ($manifest.reviewEntrySha256.Contains($entry.FullName)) {
                $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($decoded.ToArray())).ToLowerInvariant()
                if ($hash -ne $manifest.reviewEntrySha256[$entry.FullName]) { throw 'Review evidence hash mismatch' }
            }
        } finally { $decoded.Dispose(); $inputStream.Dispose() }
    }
} finally { $archive.Dispose() }
[IO.File]::WriteAllText((Join-Path $output $questionName), $question, [Text.UTF8Encoding]::new($false))
$zipHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS.txt'), "$zipHash  $archiveName`n", [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath $zipPath -Destination (Join-Path $download $archiveName)
Copy-Item -LiteralPath (Join-Path $output $questionName) -Destination (Join-Path $download $questionName)
if ((Get-FileHash -LiteralPath (Join-Path $download $archiveName) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $zipHash) { throw 'Downloads copy mismatch' }
[pscustomobject]@{ sourceRevision = $snapshot; sourceFiles = $paths.Count; evidenceEvents = $counts; bytes = (Get-Item -LiteralPath $zipPath).Length; sha256 = $zipHash; verified = $true } | ConvertTo-Json -Compress
