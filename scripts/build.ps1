#requires -Version 7
<#
.SYNOPSIS
  Build mtok for one or more platforms and emit checksums.

.EXAMPLE
  .\scripts\build.ps1
  .\scripts\build.ps1 -Targets windows/amd64,linux/amd64,darwin/arm64 -Version 0.1.0

.NOTES
  Output: dist/mtok_<version>_<os>_<arch>[.exe] plus dist/checksums.txt
  bin/mtok.exe is refreshed for local use.
#>
param(
  [string[]]$Targets = @("$($env:GOOS)/$($env:GOARCH)"),
  [string]$Version = "",
  [string]$OutputDir = "",
  [switch]$NoLocal
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  if (-not $Version) {
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
      $desc = (& git describe --tags --always --dirty 2>$null)
      if ($desc) { $Version = $desc.TrimStart('v') } else { $Version = '0.1.0-dev' }
    } else {
      $Version = '0.1.0-dev'
    }
  }
  $commit = 'none'
  if (Get-Command git -ErrorAction SilentlyContinue) {
    $c = (& git rev-parse --short HEAD 2>$null)
    if ($c) { $commit = $c.Trim() }
  }
  $date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')

  $dist = if ($OutputDir) { $OutputDir } else { Join-Path $root 'dist' }
  New-Item -ItemType Directory -Force -Path $dist | Out-Null
  New-Item -ItemType Directory -Force -Path (Join-Path $root 'bin') | Out-Null

  $ldflags = "-s -w -X main.Version=$Version -X main.Commit=$commit -X main.Date=$date"
  $hashes = @()

  foreach ($t in $Targets) {
    $parts = $t.Split('/')
    if ($parts.Count -ne 2) { throw "bad target '$t' (want os/arch)" }
    $os, $arch = $parts[0], $parts[1]
    $ext = if ($os -eq 'windows') { '.exe' } else { '' }
    $name = "mtok_${Version}_${os}_${arch}$ext"
    $out = Join-Path $dist $name

    $env:CGO_ENABLED = '0'
    $env:GOOS = $os
    $env:GOARCH = $arch
    Write-Host "build $os/$arch -> $name"
    & go build -trimpath -ldflags $ldflags -o $out ./cmd/mtok
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $os/$arch" }

    $hash = (Get-FileHash -Algorithm SHA256 -Path $out).Hash.ToLower()
    $size = [math]::Round((Get-Item $out).Length / 1MB, 2)
    $hashes += "{0}  {1}" -f $hash, $name
    Write-Host ("  {0} MB  sha256 {1}" -f $size, $hash)

    if (-not $NoLocal -and $os -eq 'windows' -and $arch -eq 'amd64') {
      Copy-Item $out (Join-Path $root 'bin/mtok.exe') -Force
    }
  }

  $checksumPath = Join-Path $dist 'checksums.txt'
  $hashes | Set-Content -Path $checksumPath -Encoding utf8
  Write-Host "checksums -> $checksumPath"
}
finally {
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  Pop-Location
}
