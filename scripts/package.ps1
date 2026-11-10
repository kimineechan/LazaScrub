param(
    # When given, the tag must match VERSION, such as v1.0.0.
    [string]$Tag = ''
)

# Packages the built programs and the documentation as a GitHub release asset.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

$version = (Get-Content 'VERSION' -Raw).Trim()
if ($version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?$') {
    throw 'VERSION must contain a semantic version such as 1.0.0.'
}
if ($Tag -and $Tag -ne "v$version") {
    throw "The release tag must match VERSION (expected v$version)."
}

$files = @(
    @{ Name = 'LazaScrub.exe';                Source = 'dist/LazaScrub.exe' },
    @{ Name = 'lazascrub-cli.exe';            Source = 'dist/lazascrub-cli.exe' },
    @{ Name = 'README.md';                    Source = 'README.md' },
    @{ Name = 'LICENSE';                      Source = 'LICENSE' },
    @{ Name = 'THIRD-PARTY-NOTICES.txt';      Source = 'THIRD-PARTY-NOTICES.txt' },
    @{ Name = 'VERSION';                      Source = 'VERSION' },
    @{ Name = 'CHANGELOG.md';                 Source = 'CHANGELOG.md' },
    @{ Name = 'VALIDATION.md';                Source = 'VALIDATION.md' },
    @{ Name = 'docs/USAGE.md';                Source = 'docs/USAGE.md' },
    @{ Name = 'docs/FORMATS.md';              Source = 'docs/FORMATS.md' },
    @{ Name = "docs/releases/v$version.md";   Source = "docs/releases/v$version.md" },
    @{ Name = 'docs/screenshots/inspect.png'; Source = 'docs/screenshots/inspect.png' },
    @{ Name = 'docs/screenshots/clean.png';   Source = 'docs/screenshots/clean.png' },
    @{ Name = 'assets/logo.svg';              Source = 'assets/logo.svg' }
)
foreach ($file in $files) {
    if (-not (Test-Path $file.Source -PathType Leaf)) { throw "Missing release input: $($file.Source)" }
}
foreach ($program in 'dist/LazaScrub.exe', 'dist/lazascrub-cli.exe') {
    if ((Get-Item $program).Length -eq 0) { throw "$program is empty. Build the app first." }
}

function Get-Sha256([string]$Path) {
    return (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

$programs = @(
    "$(Get-Sha256 'dist/LazaScrub.exe')  LazaScrub.exe",
    "$(Get-Sha256 'dist/lazascrub-cli.exe')  lazascrub-cli.exe"
)

$stage = Join-Path 'dist' 'package'
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
$root = Join-Path $stage 'LazaScrub'
foreach ($file in $files) {
    $target = Join-Path $root $file.Name
    New-Item -ItemType Directory -Force (Split-Path $target -Parent) | Out-Null
    Copy-Item $file.Source $target
}
[IO.File]::WriteAllText((Join-Path $root 'SHA256SUMS.txt'), (($programs -join "`n") + "`n"), [Text.Encoding]::ASCII)

$zip = Join-Path 'dist' "LazaScrub-v$version-windows-x64.zip"
if (Test-Path $zip) { Remove-Item $zip -Force }
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::CreateFromDirectory((Resolve-Path $stage).Path, (Join-Path (Resolve-Path 'dist').Path "LazaScrub-v$version-windows-x64.zip"), [IO.Compression.CompressionLevel]::Optimal, $false)
Remove-Item $stage -Recurse -Force

# Open the archive again to make sure it is complete.
$archive = [IO.Compression.ZipFile]::OpenRead((Resolve-Path $zip).Path)
try {
    if ($archive.Entries.Count -ne $files.Count + 1) { throw 'The release ZIP is incomplete.' }
} finally {
    $archive.Dispose()
}

$checksums = @("$(Get-Sha256 $zip)  LazaScrub-v$version-windows-x64.zip") + $programs
[IO.File]::WriteAllText((Join-Path (Resolve-Path 'dist').Path 'SHA256SUMS.txt'), (($checksums -join "`n") + "`n"), [Text.Encoding]::ASCII)
Write-Host "Created $zip"
$checksums | ForEach-Object { Write-Host $_ }
