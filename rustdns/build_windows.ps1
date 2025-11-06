<#
Build helper for Windows to avoid linker PDB write errors (LNK1201).

Usage:
  # build debug (default):
  .\build_windows.ps1

  # build release:
  .\build_windows.ps1 -Release

This script ensures a short temporary path (C:\Temp), sets TMP/TEMP to it,
and disables debug info via RUSTFLAGS to reduce PDB generation during dev builds.
#>
param(
    [switch]$Release
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $root

if (-not (Test-Path -Path 'C:\Temp')) {
    New-Item -ItemType Directory -Path 'C:\Temp' | Out-Null
}

$env:TEMP = 'C:\Temp'
$env:TMP = 'C:\Temp'

Write-Host "Using TEMP=$($env:TEMP)"

# Reduce debug info to avoid large PDBs
$env:RUSTFLAGS = '-C debuginfo=0'

if ($Release) {
    Write-Host "Building release..."
    cargo build --release
} else {
    Write-Host "Cleaning and building (dev)..."
    cargo clean
    cargo build
}

Write-Host "Build finished."
