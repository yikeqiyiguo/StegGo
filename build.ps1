# =============================================================
# StegGo V2.2 build script (Windows / PowerShell)
# Usage:
#   .\build.ps1                 # build CLI + TUI
#   .\build.ps1 -Gui            # also build GUI (needs MinGW-w64/gcc + cgo)
#   .\build.ps1 -Termux         # also build Android Termux ARM64 package
#   .\build.ps1 -Wasm           # also build WASM browser audit
#   .\build.ps1 -Test           # build and run unit tests
#   .\build.ps1 -Version v2.2.0 # set version string
# =============================================================
param(
    [switch]$Gui,
    [switch]$Termux,
    [switch]$Wasm,
    [switch]$Test,
    [string]$Version = "2.2.0"
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
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o steggo.exe ./cmd/cli
}

Invoke-Step "Building TUI (steggo-tui.exe)" {
    go build -trimpath -ldflags "-s -w" -o steggo-tui.exe ./cmd/tui
}

if ($Gui) {
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if (-not $gcc) {
        # 自动探测便携工具链（WinLibs 等），无需手动加入 PATH
        $candidates = @(
            "$env:USERPROFILE\WinLibs\mingw64\bin\gcc.exe",
            "$env:USERPROFILE\scoop\apps\gcc\current\bin\gcc.exe",
            "C:\msys64\mingw64\bin\gcc.exe",
            "C:\msys64\ucrt64\bin\gcc.exe",
            "C:\mingw64\bin\gcc.exe"
        )
        $found = $candidates | ForEach-Object { Get-Item $_ -ErrorAction SilentlyContinue } | Select-Object -First 1
        if ($found) {
            $env:Path = $found.DirectoryName + ";" + $env:Path
            $gcc = Get-Command gcc -ErrorAction SilentlyContinue
        }
    }
    if (-not $gcc) {
        Write-Host ""
        Write-Host "[!] gcc not found - GUI (Fyne) requires cgo." -ForegroundColor Red
        Write-Host "    Install MinGW-w64, e.g.:  scoop install mingw" -ForegroundColor Yellow
        Write-Host "    or:  winget install -e --id BrechtSanders.WinLibs.POSIX.UCRT" -ForegroundColor Yellow
        Write-Host "    or:  unzip WinLibs to %USERPROFILE%\WinLibs\mingw64" -ForegroundColor Yellow
        Write-Host "    Then re-run:  .\build.ps1 -Gui" -ForegroundColor Yellow
    } else {
        if ($env:CGO_ENABLED -eq "0") {
            Write-Host "[!] CGO_ENABLED=0 detected, GUI requires cgo (OpenGL). Forcing CGO_ENABLED=1." -ForegroundColor Red
        }
        Invoke-Step "Building GUI (steggo-gui.exe, requires cgo)" {
            Push-Location cmd/gui
            try {
                go mod download
                $env:CGO_ENABLED = "1"
                go build -trimpath -ldflags "-s -w" -o ..\..\steggo-gui.exe .
            } finally {
                Pop-Location
            }
        }
    }
}

if ($Termux) {
    Invoke-Step "Building Android Termux ARM64 (steggo / steggo-tui)" {
        $env:GOOS = "linux"
        $env:GOARCH = "arm64"
        $env:CGO_ENABLED = "0"
        New-Item -ItemType Directory -Force -Path dist/termux | Out-Null
        go build -trimpath -ldflags "-s -w" -o dist/termux/steggo ./cmd/cli
        go build -trimpath -ldflags "-s -w" -o dist/termux/steggo-tui ./cmd/tui
        Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
        Write-Host "  dist/termux/steggo + steggo-tui (ARM64)" -ForegroundColor Cyan
    }
}

if ($Wasm) {
    Invoke-Step "Building WASM browser audit (dist/steggo.wasm)" {
        $env:GOOS = "js"
        $env:GOARCH = "wasm"
        $env:CGO_ENABLED = "0"
        New-Item -ItemType Directory -Force -Path dist | Out-Null
        go build -trimpath -ldflags "-s -w" -o dist/steggo.wasm ./wasm
        $goroot = go env GOROOT
        Copy-Item "$goroot\lib\wasm\wasm_exec.js" dist\wasm_exec.js -Force
        Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
        Write-Host "  dist/steggo.wasm + dist/wasm_exec.js" -ForegroundColor Cyan
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
