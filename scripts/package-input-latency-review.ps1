Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$revision = 'b1b643be5b2ee8b9d308994a4e53b87593236346'
$folder = Join-Path (Get-Location) 'dist/reviews/g118-input-latency/package'
$questionPath = Join-Path (Get-Location) 'docs/reviews/g118-input-latency-question.md'
$question = [IO.File]::ReadAllText($questionPath)
if (-not $question.Contains($revision)) { throw 'Question source revision mismatch' }
$generation = & git show ($revision + ':internal/localapi/runtime.go')
if ($LASTEXITCODE -ne 0 -or ($generation -join "`n") -notmatch 'const AgentGeneration = 118') { throw 'Source generation mismatch' }
$paths = @(& git -c core.quotepath=false ls-tree -r --name-only $revision)
if ($LASTEXITCODE -ne 0 -or $paths.Count -lt 1) { throw 'Missing committed source tree' }
foreach ($path in $paths) {
  if ($path -match '(^|/)(\.git|\.cache|node_modules|dist|vendor|diagnostics)(/|$)' -or $path -match '(?i)(\.(pem|key|pfx|p12|exe|jsonl)$|(^|/)\.env($|\.))' -or $path -match '(^/|(^|/)\.\.(/|$)|\\)') { throw ('Excluded source path: ' + $path) }
}
if (Test-Path -LiteralPath $folder) { throw 'Review output already exists; do not overwrite' }
New-Item -ItemType Directory -Path $folder | Out-Null
$zipPath = Join-Path $folder 'krasis-g118-input-latency-review.zip'
& git archive --format=zip --prefix=krasis-g118/ ('--output=' + $zipPath) $revision
if ($LASTEXITCODE -ne 0) { throw 'Committed source archive failed' }
$sourceHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
$questionHash = (Get-FileHash -LiteralPath $questionPath -Algorithm SHA256).Hash.ToLowerInvariant()
$metadata = [ordered]@{
  sourceGeneration = 118
  sourceRevision = $revision
  sourceTree = (& git rev-parse ($revision + '^{tree}')).Trim()
  sourceFileCount = $paths.Count
  sourceArchiveSha256BeforeReviewEntries = $sourceHash
  questionSha256 = $questionHash
  purpose = 'Independent source review; runtime observations are user-reported, not an independent replay.'
  excluded = @('working-tree changes', '.git', 'builds', 'caches', 'installed dependencies', 'runtime settings', 'raw diagnostics', 'certificates and private keys')
}
if ($LASTEXITCODE -ne 0) { throw 'Source tree metadata failed' }
$manifest = $metadata | ConvertTo-Json -Depth 4
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::Open($zipPath, [IO.Compression.ZipArchiveMode]::Update)
try {
  foreach ($item in @(@{name='QUESTION.md';content=$question}, @{name='MANIFEST.json';content=$manifest})) {
    $entry = $archive.CreateEntry($item.name, [IO.Compression.CompressionLevel]::Optimal)
    $writer = [IO.StreamWriter]::new($entry.Open(), [Text.UTF8Encoding]::new($false))
    try { $writer.Write($item.content) } finally { $writer.Dispose() }
  }
} finally { $archive.Dispose() }
$archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
try {
  $expected = @($paths | ForEach-Object { 'krasis-g118/' + $_ }) + @('QUESTION.md', 'MANIFEST.json')
  $names = @($archive.Entries | Where-Object { -not $_.FullName.EndsWith('/') } | ForEach-Object { $_.FullName })
  if (@(Compare-Object ($expected | Sort-Object) ($names | Sort-Object)).Count -ne 0 -or $names.Count -ne ($names | Select-Object -Unique).Count) { throw 'Archive content drift' }
  foreach ($entry in $archive.Entries) {
    if ($entry.FullName.EndsWith('/')) { continue }
    $inputStream = $entry.Open()
    $decoded = [IO.MemoryStream]::new()
    try {
      $inputStream.CopyTo($decoded)
      if ($decoded.Length -ne $entry.Length) { throw 'Archive entry truncated' }
      if ($entry.FullName -eq 'QUESTION.md') {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $actual = [BitConverter]::ToString($sha.ComputeHash($decoded.ToArray())).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
        if ($actual -ne $questionHash) { throw 'Question bytes changed' }
      }
    } finally { $decoded.Dispose(); $inputStream.Dispose() }
  }
} finally { $archive.Dispose() }
Copy-Item -LiteralPath $questionPath -Destination (Join-Path $folder 'QUESTION.md')
[IO.File]::WriteAllText((Join-Path $folder 'MANIFEST.json'), $manifest, [Text.UTF8Encoding]::new($false))
$packageHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $folder 'SHA256SUMS.txt'), ($packageHash + '  krasis-g118-input-latency-review.zip' + "`n" + $questionHash + '  QUESTION.md' + "`n"), [Text.UTF8Encoding]::new($false))
[pscustomobject]@{sourceRevision=$revision;sourceFileCount=$paths.Count;archiveBytes=(Get-Item -LiteralPath $zipPath).Length;sha256=$packageHash;verified=$true} | ConvertTo-Json -Compress
