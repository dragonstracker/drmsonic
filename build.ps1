$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force -Path bin | Out-Null

$env:CGO_ENABLED = "0"

go build -trimpath -ldflags "-s -w" -o bin/drmsonic.exe ./cmd/drmsonic

Write-Host "built bin/drmsonic.exe"
