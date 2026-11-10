param(
    # When given, the tag must match VERSION, such as v1.0.0.
    [string]$Tag = ''
)

# Packages the built programs and the documentation as a GitHub release asset.
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Set-Location $repo

$version = (Get-Content (Join-Path $repo 'VERSION') -Raw).Trim()
if ($version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?$') {
    throw 'VERSION must contain a semantic version such as 1.0.0.'
}
if ($Tag -and $Tag -ne "v$version") {
    throw "The release tag must match VERSION (expected v$version)."
}

# Each entry is the path inside the ZIP, which is also the path in the repository
# unless a second value names a different source.
$files = @(
    @('LazaScrub.exe', 'dist/LazaScrub.exe'),
    @('lazascrub-cli.exe', 'dist/lazascrub-cli.exe'),
    @('README.md'),
    @('LICENSE'),
    @('THIRD-PARTY-NOTICES.txt'),
    @('VERSION'),
    @('CHANGELOG.md'),
    @('VALIDATION.md'),
    @('docs/USAGE.md'),
    @('docs/FORMATS.md'),
    @("docs/releases/v$version.md"),
    @('docs/screenshots/inspect.png'),
    @('docs/screenshots/clean.png'),
    @('assets/logo.svg')
)
foreach ($file in $files) {
    $source = Join-Path $repo $file[-1]
    if (-not (Test-Path $source -PathType Leaf)) { throw "Missing release input: $($file[-1])" }
    if ((Get-Item $source).Length -eq 0) { throw "$($file[-1]) is empty. Build the app first." }
}

function Get-Sha256([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

$dist = Join-Path $repo 'dist'
$programs = @(
    "$(Get-Sha256 (Join-Path $dist 'LazaScrub.exe'))  LazaScrub.exe",
    "$(Get-Sha256 (Join-Path $dist 'lazascrub-cli.exe'))  lazascrub-cli.exe"
)

# The entries are written one by one so that their names always use forward
# slashes, whichever version of PowerShell runs this script.
$name = "LazaScrub-v$version-windows-x64.zip"
$zip = Join-Path $dist $name
if (Test-Path $zip) { Remove-Item $zip -Force }
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$stream = [IO.File]::Open($zip, [IO.FileMode]::CreateNew)
try {
    $archive = New-Object IO.Compression.ZipArchive($stream, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in $files) {
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, (Join-Path $repo $file[-1]), "LazaScrub/$($file[0])", [IO.Compression.CompressionLevel]::Optimal) | Out-Null
        }
        $writer = New-Object IO.StreamWriter($archive.CreateEntry('LazaScrub/SHA256SUMS.txt').Open(), [Text.Encoding]::ASCII)
        $writer.Write(($programs -join "`n") + "`n")
        $writer.Dispose()
    } finally {
        $archive.Dispose()
    }
} finally {
    $stream.Dispose()
}

# Open the archive again to make sure it is complete.
$archive = [IO.Compression.ZipFile]::OpenRead($zip)
try {
    $names = @($archive.Entries | ForEach-Object { $_.FullName })
} finally {
    $archive.Dispose()
}
foreach ($file in $files + , @('SHA256SUMS.txt')) {
    if ($names -notcontains "LazaScrub/$($file[0])") { throw "The release ZIP lacks $($file[0])." }
}

$checksums = @("$(Get-Sha256 $zip)  $name") + $programs
[IO.File]::WriteAllText((Join-Path $dist 'SHA256SUMS.txt'), (($checksums -join "`n") + "`n"), [Text.Encoding]::ASCII)
Write-Host "Created $zip"
$checksums | ForEach-Object { Write-Host $_ }
