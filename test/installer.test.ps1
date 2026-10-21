param([Parameter(Mandatory=$true)][string]$InstallerPath, [Parameter(Mandatory=$true)][string]$Version)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'Installer execution is restricted to an ephemeral GitHub-hosted Windows runner.'
}
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid fixture version' }
$localData = [Environment]::GetFolderPath('LocalApplicationData')
$appRoot = Join-Path $localData 'Programs\XpotatoTunnel'
$installed = Join-Path $appRoot $Version
$profile = Join-Path $env:USERPROFILE 'AppData\Local\xpotato-mcp-tunnel-v2'
$registration = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel'
if ((Test-Path -LiteralPath $appRoot) -or (Test-Path -LiteralPath $profile) -or (Test-Path -LiteralPath $registration)) {
    throw 'Existing application or data found; fixture refuses to touch it.'
}
function Run-Fixture([string]$File, [string[]]$Arguments, [bool]$ExpectSuccess) {
    $process = Start-Process -FilePath $File -ArgumentList $Arguments -PassThru -WindowStyle Hidden
    if (-not $process.WaitForExit(60000)) { throw 'Fixture exceeded 60 seconds; no unrelated process stopped.' }
    if (($process.ExitCode -eq 0) -ne $ExpectSuccess) { throw "Unexpected fixture exit code: $($process.ExitCode)" }
}
$installer = (Resolve-Path -LiteralPath $InstallerPath).Path
Run-Fixture $installer @('/S') $true
Run-Fixture $installer @('/S') $true
$core = Join-Path $installed 'xpotato-tunnel.exe'
& $core package-check
if ($LASTEXITCODE -ne 0) { throw 'Installed payload invalid' }
if ((Get-ItemProperty -LiteralPath $registration).DisplayVersion -ne $Version) { throw 'Registration mismatch' }
if (Test-Path -LiteralPath $profile) { throw 'Installer unexpectedly created runtime/profile state' }
# Stage left by a terminated earlier installer must not be swept or block retry.
$foreignStage = Join-Path $appRoot 'stage-from-another-fixture'
New-Item -ItemType Directory -Path $foreignStage | Out-Null
$foreignFile = Join-Path $foreignStage 'keep.txt'
Set-Content -LiteralPath $foreignFile -Value 'unrelated stage'
Run-Fixture $installer @('/S') $true
if (-not (Test-Path -LiteralPath $foreignFile)) { throw 'Unrelated stage removed' }
# A nonsecret synthetic wrapper record. No Setup/Vault/runtime is used.
$bin = Join-Path $profile 'bin'
New-Item -ItemType Directory -Path $bin | Out-Null
$wrapper = Join-Path $bin 'xpotato-tunnel.cmd'
[IO.File]::WriteAllText($wrapper, 'fixture wrapper, never executed')
$expected = (Get-FileHash -LiteralPath $wrapper -Algorithm SHA256).Hash.ToLowerInvariant()
$record = @{Owner=('windows:' + [Security.Principal.WindowsIdentity]::GetCurrent().User.Value); Files=@{$wrapper=$expected}}
$record | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $profile 'command-install.json') -Encoding utf8NoBOM
$sentinel = Join-Path $profile 'retained-profile.json'
Set-Content -LiteralPath $sentinel -Value 'nonsecret retained fixture'
[IO.File]::WriteAllText($wrapper, 'modified wrapper')
$uninstallCopy = Join-Path $env:RUNNER_TEMP 'xpotato-uninstaller-fixture.exe'
Copy-Item -LiteralPath (Join-Path $installed 'Uninstall.exe') -Destination $uninstallCopy
$uninstallArgs = @('/S', ('_?=' + $installed)) # NSIS requires final _?= path unquoted.
Run-Fixture $uninstallCopy $uninstallArgs $false
if (-not (Test-Path -LiteralPath $wrapper) -or -not (Test-Path -LiteralPath $registration)) { throw 'Modified wrapper or registration lost' }
if ([IO.File]::ReadAllText($wrapper) -ne 'modified wrapper') { throw 'Modified wrapper contents changed' }
[IO.File]::WriteAllText($wrapper, 'fixture wrapper, never executed')
# Hold only our own core fixture open to simulate an in-use application file.
$handle = [IO.File]::Open($core, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
try { Run-Fixture $uninstallCopy $uninstallArgs $false } finally { $handle.Dispose() }
if (-not (Test-Path -LiteralPath $core) -or -not (Test-Path -LiteralPath $registration)) { throw 'In-use program or registration lost' }
if (Test-Path -LiteralPath $wrapper) { throw 'In-use case did not reach removal of verified wrappers' }
Run-Fixture $uninstallCopy $uninstallArgs $true
if ((Test-Path -LiteralPath $installed) -or (Test-Path -LiteralPath $registration) -or (Test-Path -LiteralPath $wrapper)) { throw 'Owned application removal incomplete' }
if (-not (Test-Path -LiteralPath $sentinel) -or -not (Test-Path -LiteralPath $foreignFile)) { throw 'Retained data removed' }
Write-Output 'Installer fixture PASS: new, repeat, stale stage, modified wrapper, in-use file, uninstall, retained data.'
