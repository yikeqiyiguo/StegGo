# =============================================================
# StegGo V1.0 build script (Windows / PowerShell)
# Usage:
#   .\build.ps1                 # build CLI + TUI
#   .\build.ps1 -Gui            # also build GUI (needs MinGW-w64/gcc + cgo)
#   .\build.ps1 -Test           # build and run unit tests
#   .\build.ps1 -Version v1.0.0 # set version string
# =============================================================
param(
    [switch]$Gui,
    [switch]$Test,
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

Write-Host "==> StegGo $Version build started" -ForegroundColor Cyan

function Invoke-Step {
    param([string]$Name, [scriptblock]$Action)
    Write-Host "==> $Name" -ForegroundColor Yellow
    & $Action
    if ($LASTEXITCODE -ne 0) { throw "$Name failed" }
}

Invoke-Step "Building CLI (steggo.exe)" {
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o steggo.exe ./cmd/steggo
}

Invoke-Step "Building TUI (steggo-tui.exe)" {
    go build -trimpath -ldflags "-s -w" -o steggo-tui.exe ./cmd/steggo-tui
}

if ($Gui) {
    if ($env:CGO_ENABLED -eq "0") {
        Write-Host "[!] CGO_ENABLED=0 detected, GUI requires cgo (OpenGL). Forcing CGO_ENABLED=1." -ForegroundColor Red
        $env:CGO_ENABLED = "1"
    }
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if (-not $gcc) {
        throw "gcc not found. Install MinGW-w64 (e.g. 'scoop install mingw') and retry, or drop -Gui."
    }
    Invoke-Step "Building GUI (steggo-gui.exe, requires cgo)" {
        Push-Location cmd/steggo-gui
        try {
            go mod download
            $env:CGO_ENABLED = "1"
            go build -trimpath -ldflags "-s -w" -o ..\..\steggo-gui.exe .
        } finally {
            Pop-Location
        }
    }
}

if ($Test) {
    Invoke-Step "Running unit tests" { go test -v ./... }
}

Write-Host ""
Write-Host "==> Build finished" -ForegroundColor Green
Get-ChildItem -Path $Root -Filter "steggo*.exe" | ForEach-Object {
    Write-Host ("    {0,-24} {1,10:N0} bytes" -f $_.Name, $_.Length)
}
