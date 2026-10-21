# Cross-compile DocxCraft release archives into dist/
param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path dist | Out-Null
Get-ChildItem dist -ErrorAction SilentlyContinue | Remove-Item -Force

$targets = @(
    @{ GOOS = "linux"; GOARCH = "amd64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "linux"; GOARCH = "arm64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "darwin"; GOARCH = "arm64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe"; Archive = "zip" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe"; Archive = "zip" }
)

$ldflags = "-s -w -X main.version=$Version"

foreach ($t in $targets) {
    $name = "docxcraft-$Version-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
    $out = Join-Path "dist" $name
    Write-Host "Building $out"
    $env:CGO_ENABLED = "0"
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    go build -ldflags $ldflags -o $out ./cmd/docxcraft
    if ($LASTEXITCODE -ne 0) { throw "build failed for $($t.GOOS)/$($t.GOARCH)" }

    if ($t.Archive -eq "zip") {
        $zip = Join-Path "dist" "docxcraft-$Version-$($t.GOOS)-$($t.GOARCH).zip"
        if (Test-Path $zip) { Remove-Item $zip -Force }
        Compress-Archive -Path $out -DestinationPath $zip
    }
    else {
        $tar = Join-Path "dist" "docxcraft-$Version-$($t.GOOS)-$($t.GOARCH).tar.gz"
        tar -C dist -czf $tar $name
        if ($LASTEXITCODE -ne 0) { throw "tar failed for $name" }
    }
    Remove-Item $out -Force
}

Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Get-ChildItem dist | Format-Table Name, Length
