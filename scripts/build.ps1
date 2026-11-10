$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Install Go from https://go.dev/dl/, reopen PowerShell, then run this script.'
}
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
New-Item -ItemType Directory -Force dist | Out-Null
go run -buildvcs=false ./tools/mkres
if ($LASTEXITCODE -ne 0) { throw 'Resource generation failed.' }
go vet -buildvcs=false ./...
if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
go test -buildvcs=false ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
go build -buildvcs=false -trimpath -ldflags '-H=windowsgui -s -w' -o dist/LazaScrub.exe ./cmd/lazascrub
if ($LASTEXITCODE -ne 0) { throw 'LazaScrub build failed.' }
go build -buildvcs=false -trimpath -ldflags '-s -w' -o dist/lazascrub-cli.exe ./cmd/lazascrub-cli
if ($LASTEXITCODE -ne 0) { throw 'Command-line tool build failed.' }
go run -buildvcs=false ./cmd/verify
if ($LASTEXITCODE -ne 0) { throw 'Release verification failed.' }
Write-Host 'Built dist\LazaScrub.exe and dist\lazascrub-cli.exe'
