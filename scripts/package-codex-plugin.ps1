param(
    [string]$DocumentRoot = (Get-Location).Path
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$root = (Resolve-Path -LiteralPath $DocumentRoot).ProviderPath
$market = Join-Path $repo 'dist/codex-docxcraft'
$plugin = Join-Path $market 'plugins/docxcraft'
$bin = Join-Path $plugin 'bin'
$catalog = Join-Path $market '.agents/plugins'
New-Item -ItemType Directory -Force -Path $bin, $catalog | Out-Null
Copy-Item -Path (Join-Path $repo 'plugins/docxcraft/*') -Destination $plugin -Recurse -Force

$mcpPath = Join-Path $plugin 'mcp.json'
$mcp = Get-Content -LiteralPath $mcpPath -Raw | ConvertFrom-Json
$mcp.mcpServers.docxcraft.args += @('--sandbox-dir', $root)

$marketplace = @{
    name = 'docxcraft-local'
    interface = @{ displayName = 'DocxCraft Local' }
    plugins = @(@{
        name = 'docxcraft'
        source = @{ source = 'local'; path = './plugins/docxcraft' }
        policy = @{ installation = 'AVAILABLE'; authentication = 'ON_INSTALL' }
    })
}
$catalogPath = Join-Path $catalog 'marketplace.json'
$utf8 = [System.Text.UTF8Encoding]::new($false)
[IO.File]::WriteAllText($catalogPath, ($marketplace | ConvertTo-Json -Depth 10), $utf8)
[IO.File]::WriteAllText($mcpPath, ($mcp | ConvertTo-Json -Depth 10), $utf8)

Push-Location -LiteralPath $repo
$previousCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    go build -buildvcs=false -trimpath -o (Join-Path $bin 'docxcraft.exe') ./cmd/docxcraft
    if ($LASTEXITCODE -ne 0) { throw 'DocxCraft build failed.' }
} finally {
    $env:CGO_ENABLED = $previousCgo
    Pop-Location
}
Write-Output $market
