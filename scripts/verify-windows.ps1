# This helper is executed only AFTER the Release-body block checks the outer ZIP digest.
param([Parameter(Mandatory = $true)][byte[]] $ArchiveBytes,
      [Parameter(Mandatory = $true)][string] $DestinationPath,
      [ValidatePattern('^\d+\.\d+\.\d+$')][string] $ExpectedVersion,
      [ValidatePattern('^[0-9a-f]{40}$')][string] $ExpectedCommit)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Assert-SafeName([string] $Name) {
    if ($Name -cnotmatch '^[a-zA-Z0-9._/-]+$' -or $Name.StartsWith('/') -or
        @($Name.Split('/') | Where-Object { $_ -in @('', '.', '..') }).Count) {
        throw "Unsafe ZIP path: $Name"
    }
    foreach ($part in $Name.Split('/')) {
        if ($part.EndsWith('.') -or $part -imatch '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\.|$)') {
            throw "Unsafe Windows path: $Name"
        }
    }
}
function Assert-Content([string] $Root, $Entries, [string] $Manifest, $Checksums) {
    $actualFiles = @{}
    foreach ($file in Get-ChildItem -LiteralPath $Root -Recurse -Force) {
        if ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Reparse points are not allowed' }
        if (-not $file.PSIsContainer) {
            $relative = $file.FullName.Substring($Root.Length + 1).Replace('\', '/')
            if (-not $Entries.ContainsKey($relative) -or $Entries[$relative].FullName -cne $relative) { throw "Unexpected file: $relative" }
            $actualFiles[$relative] = $true
        }
    }
    if ($actualFiles.Count -ne $Entries.Count) { throw 'Missing package files' }
    if ([IO.File]::ReadAllText((Join-Path $Root 'SHA256SUMS')) -cne $Manifest) { throw 'Manifest differs from verified ZIP' }
    foreach ($name in $Checksums.Keys) {
        if ((Get-FileHash -LiteralPath (Join-Path $Root $name) -Algorithm SHA256).Hash -ine $Checksums[$name]) { throw "Inner SHA256 mismatch: $name" }
    }
    $info = [IO.File]::ReadAllText((Join-Path $Root 'build-info.json')) | ConvertFrom-Json
    if ($info.version -isnot [string] -or $info.commit -isnot [string] -or
        $info.platform -isnot [string] -or $info.binary -isnot [string] -or
        $info.version -cne $ExpectedVersion -or $info.commit -cne $ExpectedCommit -or
        $info.platform -cne 'windows-amd64' -or $info.dirty -isnot [bool] -or $info.dirty -ne $false -or
        $info.binary -cne 'xpotato-tunnel.exe') { throw 'Build version/source/platform/dirty mismatch' }
}
$memory = [IO.MemoryStream]::new($ArchiveBytes, $false)
$zip = [IO.Compression.ZipArchive]::new($memory, [IO.Compression.ZipArchiveMode]::Read)
$stage = $null
try {
    $entries = @{}
    foreach ($entry in $zip.Entries) {
        Assert-SafeName $entry.FullName
        if ($entries.ContainsKey($entry.FullName)) { throw 'Duplicate or case-colliding ZIP entries' }
        # Reject Unix symlinks, directories and other special files (regular/unspecified only).
        $kind = ($entry.ExternalAttributes -shr 16) -band 0xF000
        if ($kind -notin @(0, 0x8000) -or ($entry.ExternalAttributes -band 0x10)) { throw 'ZIP must contain only regular files' }
        $entries[$entry.FullName] = $entry
    }
    foreach ($required in @('SHA256SUMS', 'xpotato-tunnel.exe', 'build-info.json', 'verify-windows.ps1')) {
        if (-not $entries.ContainsKey($required) -or $entries[$required].FullName -cne $required) { throw "Required file missing: $required" }
    }
    $reader = [IO.StreamReader]::new($entries['SHA256SUMS'].Open())
    try { $manifest = $reader.ReadToEnd() } finally { $reader.Dispose() }
    $checksums = @{}
    foreach ($line in ($manifest -split '\r?\n' | Where-Object { $_.Length })) {
        if ($line -cnotmatch '^([0-9a-fA-F]{64})  (.+)$') { throw 'Invalid checksum line' }
        $hash = $Matches[1]; $name = $Matches[2]
        Assert-SafeName $name
        if ($name -ieq 'SHA256SUMS' -or $checksums.ContainsKey($name) -or
            -not $entries.ContainsKey($name) -or $entries[$name].FullName -cne $name) { throw 'Invalid checksum inventory' }
        $checksums[$name] = $hash
    }
    if ($checksums.Count -ne $entries.Count - 1) { throw 'Checksum inventory does not match ZIP' }
    $destination = [IO.Path]::GetFullPath($DestinationPath).TrimEnd('\', '/')
    if (Test-Path -LiteralPath $destination) {
        $item = Get-Item -LiteralPath $destination -Force
        if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Destination must be a regular directory' }
        Assert-Content $destination $entries $manifest $checksums
    } else {
        $parent = [IO.Path]::GetDirectoryName($destination)
        [IO.Directory]::CreateDirectory($parent) | Out-Null
        $stage = Join-Path $parent ('.xpotato-verify-' + [guid]::NewGuid().ToString('N'))
        [IO.Directory]::CreateDirectory($stage) | Out-Null
        foreach ($entry in $entries.Values) {
            $target = Join-Path $stage $entry.FullName
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
            $inputStream = $entry.Open()
            try {
                $outputStream = [IO.File]::Open($target, [IO.FileMode]::CreateNew)
                try { $inputStream.CopyTo($outputStream) } finally { $outputStream.Dispose() }
            } finally { $inputStream.Dispose() }
        }
        Assert-Content $stage $entries $manifest $checksums
        # Unlike Move-Item, this fails if the destination appeared; never nests/overwrites.
        [IO.Directory]::Move($stage, $destination)
        $stage = $null
    }
    Write-Host "Verified: $destination (version $ExpectedVersion; source $ExpectedCommit)"
} finally {
    $zip.Dispose(); $memory.Dispose()
    # Only the exact GUID stage created by this invocation is removed.
    if ($stage -and [IO.Directory]::Exists($stage)) { [IO.Directory]::Delete($stage, $true) }
}
