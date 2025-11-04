# PowerShell build script for Windows developers
param(
    [string]$Target = ""
)
$ErrorActionPreference = 'Stop'
$Top = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $Top\rustdns
Write-Output "Building rustdns (host)..."
cargo build --release

$libdir = Join-Path $Top "rustdns/target/release"
if (-Not (Test-Path (Join-Path $libdir "librustdns.a"))) {
    Write-Warning "Static lib not found; linking may use dynamic lib instead"
}

Set-Location $Top
$env:CGO_ENABLED = "1"
$env:CGO_LDFLAGS = "-L$libdir -lrustdns"
Write-Output "Building Go binary..."
go build -v -o piblock.exe
Write-Output "Built piblock.exe"

if ($Target -ne "") {
    Write-Output "Cross-building rustdns for target $Target"
    rustup target add $Target
    Set-Location $Top\rustdns
    cargo build --release --target $Target
    Write-Output "Cross build completed"
}
