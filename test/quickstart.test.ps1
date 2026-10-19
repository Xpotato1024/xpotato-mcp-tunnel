param([Parameter(Mandatory = $true)][string] $BinaryPath, [string] $RepositoryRoot)
$ErrorActionPreference = 'Stop'
$repo = if ($RepositoryRoot) { [IO.Path]::GetFullPath($RepositoryRoot) } else { Split-Path -Parent $PSScriptRoot }
$binary = (Resolve-Path -LiteralPath $BinaryPath).Path
$document = Get-Content -LiteralPath (Join-Path $repo 'docs/quickstart.md') -Raw -Encoding UTF8
$example = [regex]::Match($document, '(?s)```powershell\r?\n(.*?)```').Groups[1].Value
if (-not $example) { throw 'Windows quickstart example not found' }
$tokens = $null
$parseErrors = $null
$syntax = [System.Management.Automation.Language.Parser]::ParseInput($example, [ref] $tokens, [ref] $parseErrors)
if ($parseErrors.Count) { throw 'Quickstart example has parse errors' }
$singleBlock = $null
if ($syntax.EndBlock.Statements.Count -eq 1) {
    $pipeline = $syntax.EndBlock.Statements[0]
    if ($pipeline -is [System.Management.Automation.Language.PipelineAst]) {
        $command = $pipeline.PipelineElements[0]
        if ($command -is [System.Management.Automation.Language.CommandAst] -and
            $command.InvocationOperator -eq [System.Management.Automation.Language.TokenKind]::Ampersand -and
            $command.CommandElements[0] -is [System.Management.Automation.Language.ScriptBlockExpressionAst]) {
            $singleBlock = $command.CommandElements[0].ScriptBlock.GetScriptBlock()
        }
    }
}
$temporaryBase = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$fixtureRoot = Join-Path $temporaryBase ('xpotato-quickstart-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
$content = Join-Path $fixtureRoot 'content'
New-Item -ItemType Directory -Path $content | Out-Null
Copy-Item -LiteralPath $binary -Destination (Join-Path $content 'xpotato-tunnel.exe')
[IO.File]::WriteAllText((Join-Path $content 'README.md'), 'fixture payload')
[IO.File]::WriteAllText((Join-Path $content 'build-info.json'), '{"version":"0.2.9","platform":"windows-amd64"}')
$sums = @('README.md', 'build-info.json', 'xpotato-tunnel.exe') | ForEach-Object {
    '{0}  {1}' -f (Get-FileHash -LiteralPath (Join-Path $content $_) -Algorithm SHA256).Hash, $_
}
[IO.File]::WriteAllText((Join-Path $content 'SHA256SUMS'), ($sums -join "`n") + "`n")
$template = Join-Path $fixtureRoot 'template.zip'
Microsoft.PowerShell.Archive\Compress-Archive -Path (Join-Path $content '*') -DestinationPath $template
$failures = @()
$script:expandCalls = 0
function Expand-Archive {
    [CmdletBinding()]
    param([string] $LiteralPath, [string] $DestinationPath)
    $script:expandCalls++
    Microsoft.PowerShell.Archive\Expand-Archive @PSBoundParameters
}
function Invoke-DocumentedExample {
    param([string] $ArchivePath, [string] $DestinationPath, [switch] $VerifyExisting)
    # Model stale interactive state from a previous failed attempt.
    $archiveHash = '0' * 64
    if ($singleBlock) {
        & $singleBlock -ArchivePath $ArchivePath -DestinationPath $DestinationPath -VerifyExisting:$VerifyExisting
    } else {
        # Red-test the old example as lines/statements pasted into an interactive prompt.
        $legacy = $example.Replace('$HOME\Downloads\xpotato-tunnel-0.2.9-windows-amd64.zip', $ArchivePath).
            Replace('$HOME\Apps\xpotato-tunnel-0.2.9', $DestinationPath)
        $legacyErrors = $null
        $legacyTokens = $null
        $legacySyntax = [System.Management.Automation.Language.Parser]::ParseInput($legacy, [ref] $legacyTokens, [ref] $legacyErrors)
        foreach ($statement in $legacySyntax.EndBlock.Statements) {
            try { . ([scriptblock]::Create($statement.Extent.Text)) } catch { }
        }
    }
}
function Test-Case {
    param([string] $Name, [string] $Mode, [bool] $ShouldSucceed, [int] $ExpectedExpansions,
          [switch] $Existing, [switch] $VerifyExisting)
    $case = Join-Path $fixtureRoot $Name
    New-Item -ItemType Directory -Path $case | Out-Null
    $archive = Join-Path $case 'xpotato-tunnel-0.2.9-windows-amd64.zip'
    $destination = Join-Path $case 'expanded'
    Copy-Item -LiteralPath $template -Destination $archive
    if ($Mode -eq 'wrong-download-name') {
        $renamed = Join-Path $case 'xpotato-tunnel-0.2.9-windows-amd64 (1).zip'
        Move-Item -LiteralPath $archive -Destination $renamed
        $archive = $renamed
    }
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
    [IO.File]::WriteAllText(($archive + '.sha256'), ($hash + '  ' + [IO.Path]::GetFileName($archive) + "`n"))
    if ($Existing) {
        Microsoft.PowerShell.Archive\Expand-Archive -LiteralPath $archive -DestinationPath $destination
        [IO.File]::WriteAllText((Join-Path $destination 'keep.txt'), 'keep existing files')
    }
    switch ($Mode) {
        'missing-archive' { Remove-Item -LiteralPath $archive }
        'missing-checksum' { Remove-Item -LiteralPath ($archive + '.sha256') }
        'text-extension' { Move-Item -LiteralPath ($archive + '.sha256') -Destination ($archive + '.sha256.txt') }
        'hash-mismatch' { [IO.File]::WriteAllText(($archive + '.sha256'), (('0' * 64) + '  ' + [IO.Path]::GetFileName($archive))) }
        'wrong-checksum-name' { [IO.File]::WriteAllText(($archive + '.sha256'), ($hash + '  another.zip')) }
        'malformed-checksum' { [IO.File]::WriteAllText(($archive + '.sha256'), 'not a checksum') }
        'corrupt-existing-file' { [IO.File]::WriteAllText((Join-Path $destination 'README.md'), 'corrupted') }
        'self-consistent-tamper' {
            [IO.File]::WriteAllText((Join-Path $destination 'README.md'), 'changed with matching local checksum')
            $changedHash = (Get-FileHash -LiteralPath (Join-Path $destination 'README.md') -Algorithm SHA256).Hash
            $manifest = [IO.File]::ReadAllText((Join-Path $destination 'SHA256SUMS'))
            $manifest = [regex]::Replace($manifest, '(?m)^[0-9a-fA-F]{64}  README.md$', ($changedHash + '  README.md'))
            [IO.File]::WriteAllText((Join-Path $destination 'SHA256SUMS'), $manifest)
        }
    }
    $script:expandCalls = 0
    $threw = $false
    Push-Location -LiteralPath $case
    try {
        try { Invoke-DocumentedExample -ArchivePath $archive -DestinationPath $destination -VerifyExisting:$VerifyExisting | Out-Null }
        catch { $threw = $true }
    } finally { Pop-Location }
    $errors = @()
    if ($threw -eq $ShouldSucceed) { $errors += 'wrong success/failure result' }
    if ($script:expandCalls -ne $ExpectedExpansions) { $errors += "Expand-Archive called $script:expandCalls times" }
    if (-not $Existing -and -not $ShouldSucceed -and (Test-Path -LiteralPath $destination)) { $errors += 'destination created after failed validation' }
    if ($Existing -and (Get-Content -LiteralPath (Join-Path $destination 'keep.txt') -Raw) -ne 'keep existing files') { $errors += 'existing file changed' }
    if ($errors.Count) { $script:failures += ($Name + ': ' + ($errors -join '; ')) }
    else { Write-Host ('PASS ' + $Name) }
}
Test-Case 'missing-archive' 'missing-archive' $false 0
Test-Case 'missing-checksum' 'missing-checksum' $false 0
Test-Case 'checksum-saved-as-text' 'text-extension' $false 0
Test-Case 'archive-hash-mismatch' 'hash-mismatch' $false 0
Test-Case 'wrong-checksum-filename' 'wrong-checksum-name' $false 0
Test-Case 'malformed-checksum' 'malformed-checksum' $false 0
Test-Case 'renamed-download' 'wrong-download-name' $false 0
Test-Case 'existing-destination' 'valid' $false 0 -Existing
Test-Case 'existing-missing-checksum' 'missing-checksum' $false 0 -Existing -VerifyExisting
Test-Case 'existing-internal-mismatch' 'corrupt-existing-file' $false 0 -Existing -VerifyExisting
Test-Case 'existing-payload-and-manifest-changed' 'self-consistent-tamper' $false 0 -Existing -VerifyExisting
Test-Case 'valid-existing' 'valid' $true 0 -Existing -VerifyExisting
Test-Case 'valid-new' 'valid' $true 1
if (-not $singleBlock) { $failures += 'example is not one script-block invocation' }
if ($failures.Count) { throw (($failures -join "`n") + "`nFixtures retained: " + $fixtureRoot) }
# Only this test's newly-created GUID directory may be removed.
$resolvedFixture = [IO.Path]::GetFullPath($fixtureRoot)
if (-not $resolvedFixture.StartsWith($temporaryBase, [StringComparison]::OrdinalIgnoreCase) -or
    (Split-Path -Leaf $resolvedFixture) -notmatch '^xpotato-quickstart-[0-9a-f]{32}$') { throw 'Unsafe fixture cleanup path' }
Remove-Item -LiteralPath $resolvedFixture -Recurse
Write-Host 'PASS all 13 quickstart regression cases'
