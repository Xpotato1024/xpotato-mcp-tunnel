param([string] $BinaryPath, [string] $RepositoryRoot)
$ErrorActionPreference = 'Stop'
$repo = if ($RepositoryRoot) { [IO.Path]::GetFullPath($RepositoryRoot) } else { Split-Path -Parent $PSScriptRoot }
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('xpotato-quickstart-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($fixtureRoot) | Out-Null
$version = '0.2.10'; $commit = 'a' * 40
$template = [IO.File]::ReadAllText((Join-Path $repo 'scripts/release-windows.ps1.in'))
$helper = [IO.File]::ReadAllBytes((Join-Path $repo 'scripts/verify-windows.ps1'))
$script:cancelHash = $false
$script:downloadCalls = 0
function Get-FileHash {
    param([string] $LiteralPath, [string] $Algorithm)
    if ($script:cancelHash) { throw [OperationCanceledException]::new('fixture cancellation') }
    Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $LiteralPath -Algorithm $Algorithm
}
function Invoke-RestMethod {
    param([string] $Uri, [int] $TimeoutSec)
    if ($Uri -notlike 'https://api.github.com/repos/Xpotato1024/xpotato-mcp-tunnel/*') { throw 'Wrong API repository' }
    if ($script:mode -eq 'api-failure') { throw 'fixture API failure' }
    if ($Uri -like '*/releases/tags/v0.2.10') { return $script:release }
    if ($Uri -like '*/git/ref/tags/v0.2.10') {
        $sha = if ($script:mode -eq 'wrong-tag') { 'c' * 40 } else { $commit }
        return [pscustomobject]@{ object = [pscustomobject]@{ type = 'commit'; sha = $sha } }
    }
    throw 'Unexpected API request'
}
function Invoke-WebRequest {
    param([switch] $UseBasicParsing, [string] $Uri, [string] $OutFile, [int] $TimeoutSec)
    $script:downloadCalls++
    if ($Uri -cne $script:release.assets[0].browser_download_url) { throw 'Wrong download URL' }
    if ($script:mode -eq 'download-failure') { throw 'fixture HTTP failure' }
    if ($script:mode -eq 'download-cancel') { throw [OperationCanceledException]::new('fixture cancel') }
    $data = $script:zipBytes
    if ($script:mode -eq 'partial-download') { $data = $data[0..9] }
    if ($script:mode -eq 'html-download') { $data = [Text.Encoding]::UTF8.GetBytes('<html>login</html>') }
    if ($script:mode -eq 'hash-mismatch') { $data = $data.Clone(); $data[5] = $data[5] -bxor 1 }
    [IO.File]::WriteAllBytes($OutFile, $data)
}
function Bytes([string] $Text) { ,([Text.Encoding]::UTF8.GetBytes($Text)) }
function Hash([byte[]] $Data) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { [BitConverter]::ToString($sha.ComputeHash($Data)).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
}
function Make-Zip([string] $Mode) {
    $files = [ordered]@{
        'README.md' = (Bytes 'fixture')
        'xpotato-tunnel.exe' = (Bytes 'Not executable; verification must never execute it.')
        'build-info.json' = (Bytes ('{"version":"' + $version + '","commit":"' + $commit + '","platform":"windows-amd64","dirty":false,"binary":"xpotato-tunnel.exe"}'))
        'verify-windows.ps1' = $helper
    }
    if ($Mode -eq 'wrong-version') { $files['build-info.json'] = Bytes '{"version":"0.2.9"}' }
    if ($Mode -eq 'dirty') { $files['build-info.json'] = Bytes ('{"version":"' + $version + '","commit":"' + $commit + '","platform":"windows-amd64","dirty":true,"binary":"xpotato-tunnel.exe"}') }
    if ($Mode -eq 'wrong-commit') { $files['build-info.json'] = Bytes ('{"version":"' + $version + '","commit":"' + ('b' * 40) + '","platform":"windows-amd64","dirty":false,"binary":"xpotato-tunnel.exe"}') }
    if ($Mode -like 'metadata-*') {
        $info = [ordered]@{version=$version;commit=$commit;platform='windows-amd64';dirty=$false;binary='xpotato-tunnel.exe'}
        $field = $Mode.Substring('metadata-'.Length)
        $info[$field] = @()
        $files['build-info.json'] = Bytes (ConvertTo-Json -InputObject $info)
    }
    $manifest = ''
    foreach ($key in $files.Keys) { $manifest += (Hash $files[$key]) + '  ' + $key + "`n" }
    if ($Mode -eq 'duplicate-manifest') { $manifest += (Hash $files['README.md']) + "  readme.md`n" }
    if ($Mode -eq 'inner-mismatch') { $manifest = $manifest.Replace((Hash $files['README.md']), ('0' * 64)) }
    if ($Mode -eq 'missing-inventory') { $manifest = ($manifest -split "`n" | Where-Object { $_ -notlike '*README.md' }) -join "`n" }
    if ($Mode -eq 'malformed-manifest') { $manifest = 'bad' }
    $files['SHA256SUMS'] = Bytes $manifest
    if ($Mode -eq 'extra-file') { $files['extra.txt'] = Bytes 'unlisted' }
    if ($Mode -eq 'missing-manifest') { $files.Remove('SHA256SUMS') }
    $memory = [IO.MemoryStream]::new()
    $zip = [IO.Compression.ZipArchive]::new($memory, [IO.Compression.ZipArchiveMode]::Create, $true)
    try {
        $names = @($files.Keys)
        $additional = switch ($Mode) {
            'duplicate-entry' { 'README.md' }; 'case-collision' { 'readme.md' }
            'unsafe-parent' { '../escape' }; 'unsafe-drive' { 'C:/escape' }
            'unsafe-backslash' { 'bad\escape' }; 'unsafe-device' { 'NUL.txt' }
        }
        if ($additional) { $names += $additional }
        foreach ($name in $names) {
            $entry = $zip.CreateEntry($name)
            $data = if ($files.Contains($name)) { $files[$name] } else { Bytes 'bad' }
            $stream = $entry.Open()
            try { $stream.Write($data, 0, $data.Length) } finally { $stream.Dispose() }
        }
    } finally { $zip.Dispose() }
    try { ,$memory.ToArray() } finally { $memory.Dispose() }
}
function Block([byte[]] $Data) {
    $text = $template.Replace('@@VERSION@@', $version).Replace('@@COMMIT@@', $commit).
        Replace('@@SHA256@@', (Hash $Data)).Replace('@@SIZE@@', [string] $Data.Length).
        Replace('@@ASSET_ID@@', '456').Replace('@@RELEASE_ID@@', '123')
    $script:release = [pscustomobject]@{
        id = 123; tag_name = 'v0.2.10'; draft = $false; prerelease = $false
        html_url = 'https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10'
        assets = @([pscustomobject]@{ id = 456; name = 'xpotato-tunnel-0.2.10-windows-amd64.zip'; state = 'uploaded'; size = $Data.Length; digest = ('sha256:' + (Hash $Data)); browser_download_url = 'https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-windows-amd64.zip' })
    }
    $tokens = $null; $errors = $null
    $ast = [Management.Automation.Language.Parser]::ParseInput($text, [ref] $tokens, [ref] $errors)
    if ($errors.Count -or $ast.EndBlock.Statements.Count -ne 1) { throw 'Rendered block must be one invocation' }
    $ast.EndBlock.Statements[0].PipelineElements[0].CommandElements[0].ScriptBlock.GetScriptBlock()
}
$count = 0
try {
    foreach ($script:mode in @('valid', 'api-failure', 'wrong-tag', 'null-digest', 'duplicate-asset', 'wrong-asset', 'draft',
                              'download-failure', 'download-cancel', 'partial-download', 'html-download', 'hash-mismatch',
                              'wrong-version', 'wrong-commit', 'dirty', 'duplicate-manifest', 'inner-mismatch',
                              'missing-inventory', 'malformed-manifest', 'extra-file', 'missing-manifest',
                              'duplicate-entry', 'case-collision', 'unsafe-parent', 'unsafe-drive', 'unsafe-backslash',
                              'unsafe-device', 'metadata-version', 'metadata-commit', 'metadata-platform', 'metadata-binary',
                              'array-release-id', 'array-digest', 'cancel', 'existing-extra', 'existing-tamper', 'existing-manifest-tamper', 'retry')) {
        $case = Join-Path $fixtureRoot $script:mode
        [IO.Directory]::CreateDirectory($case) | Out-Null
        $destination = Join-Path $case 'expanded'
        $script:zipBytes = Make-Zip $script:mode
        $block = Block $script:zipBytes
        if ($script:mode -eq 'null-digest') { $script:release.assets[0].digest = $null }
        if ($script:mode -eq 'duplicate-asset') { $script:release.assets += $script:release.assets[0] }
        if ($script:mode -eq 'wrong-asset') { $script:release.assets[0].id = 999 }
        if ($script:mode -eq 'draft') { $script:release.draft = $true }
        if ($script:mode -eq 'array-release-id') { $script:release.id = @() }
        if ($script:mode -eq 'array-digest') { $script:release.assets[0].digest = @() }
        if ($script:mode -like 'existing-*' -or $script:mode -eq 'retry') {
            & $block -DestinationPath $destination
            if ($script:mode -eq 'existing-extra') { [IO.File]::WriteAllText((Join-Path $destination 'keep.txt'), 'keep') }
            if ($script:mode -eq 'existing-tamper') { [IO.File]::WriteAllText((Join-Path $destination 'README.md'), 'tamper') }
            if ($script:mode -eq 'existing-manifest-tamper') { [IO.File]::WriteAllText((Join-Path $destination 'SHA256SUMS'), 'tamper') }
            $before = @{}
            foreach ($file in Get-ChildItem -LiteralPath $destination -File -Recurse) { $before[$file.FullName] = Hash ([IO.File]::ReadAllBytes($file.FullName)) }
        }
        $script:cancelHash = $script:mode -eq 'cancel'
        $succeeded = $true
        try { & $block -DestinationPath $destination } catch { $succeeded = $false }
        finally { $script:cancelHash = $false }
        if ($succeeded -ne ($script:mode -in @('valid', 'retry'))) { throw "Unexpected result: $script:mode" }
        if (@(Get-ChildItem -LiteralPath $case -Force -Filter '.xpotato-verify-*').Count) { throw "Stage leaked: $script:mode" }
        if ($script:mode -like 'existing-*') {
            foreach ($file in $before.Keys) { if ((Hash ([IO.File]::ReadAllBytes($file))) -cne $before[$file]) { throw "Existing changed: $script:mode" } }
        } elseif (-not $succeeded -and (Test-Path -LiteralPath $destination)) { throw "Failed extraction published: $script:mode" }
        if ($script:mode -in @('cancel', 'partial-download', 'download-cancel', 'api-failure')) {
            $oldMode = $script:mode; $script:mode = 'retry'
            & $block -DestinationPath $destination
            $script:mode = $oldMode
        }
        $count++
        Write-Host "PASS $script:mode"
    }
    Write-Host "Passed $count release verification cases; no executable was run."
} finally {
    $tempBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not [IO.Path]::GetFullPath($fixtureRoot).StartsWith($tempBase, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe fixture cleanup' }
    [IO.Directory]::Delete($fixtureRoot, $true)
}
