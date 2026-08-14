<div align="center">

# 🔐 StegGo V2.1.1

**Fully Offline Anti-Detection Steganography Tool · Six Algorithms · Three Frontends · Hide Secrets in Images / Audio / PDF / Text / Video**

[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)]()
[![CLI](https://img.shields.io/badge/CLI-✓-brightgreen)]()
[![TUI](https://img.shields.io/badge/TUI-✓-brightgreen)]()
[![GUI](https://img.shields.io/badge/GUI-Fyne-9cf)]()
[![Docker](https://img.shields.io/badge/Docker-Pluggable%20Build-2496ED?logo=docker)]()

*"Hide it, and no one will know."*

  Documentation: [User Guide](docs/USAGE.md) · SDK: [SDK Docs](docs/SDK.md) · Disclaimer: [Disclaimer](docs/DISCLAIMER.md) · 中文版: [中文文档](README.md)

</div>

---

## Showcase

### Before vs. After Embedding (Visually Indistinguishable)

StegGo's goal is to embed secret data into carriers in encrypted form while keeping the carrier's visual/audio quality essentially identical to the original.

<table>
<tr>
<td align="center">
  <b>Original Carrier</b><br>
  <img src="assets/abu.png" width="320"><br>
  <sub>PNG · 800×800 · No steganography</sub>
</td>
<td align="center">
  <b>Carrier After Embedding</b><br>
  <img src="assets/abu-text.png" width="320"><br>
  <sub>PNG · 800×800 · 1.2 MB encrypted secret embedded (LSB 2-bit)</sub>
</td>
</tr>
</table>

> **Can you tell the difference?** That's the point of steganography — the information is hidden in the image, yet almost impossible to distinguish with the naked eye.
>
> Reference quality metrics: PSNR ≈ 52 dB (excellent), SSIM ≈ 0.998 (excellent).
> Lower bit depth (e.g., 1-bit) yields higher quality; adaptive algorithms (HUGO/WOW/UNIWARD) concentrate changes in textured regions for better concealment.

### Three Frontends

| CLI (Command Line) | TUI (Terminal UI) | GUI (Desktop GUI) |
|--------------------|-------------------|-------------------|
| 13 subcommands, script-friendly, pipelines & silent mode | Menu → form → run state machine, usable without a graphical environment | Light-green theme, tabbed interface, file browser dialogs, real-time status feedback |
| `steggo hide -c a.png -s secret.txt -p pass` | Keyboard navigation + secure password input | Hide / Extract / Watermark / Capacity / Quality / Audit / Batch / About |

> For a detailed guide see the [User Guide](docs/USAGE.md).

---

## Supported Carrier Formats

| Type | Formats | Steganography Method | Notes |
|------|---------|----------------------|-------|
| Image | `.png` `.bmp` `.tif` `.tiff` | **LSB / DCT / DWT / HUGO / WOW / UNIWARD** | Bit depth 1-4, channel mask, block size/quantization/levels adjustable |
| Audio | `.wav` `.flac` | **Trailing container** | Does not break headers or data integrity |
| Document | `.pdf` | **Trailing container** | Written before the EOF marker, rendering structure intact |
| Text | `.txt` `.md` `.markdown` | **Zero-width characters** | U+200B/U+200C encoding, invisible to the eye |
| Video | `.mp4` `.mkv` `.webm` | **Frame sharding + XOR redundancy** | Fragmented embedding + redundant error correction |
| ~~Lossy~~ | `.jpg` `.jpeg` `.mp3` `.flac` `.aac` `.ogg` `.m4a` `.wma` | **Blocked** | Lossy compression destroys payload bits — always rejected |

---

## Installation

### Option 1: Download Pre-built Binaries

Download from [Releases](https://github.com/yikeqiyiguo/StegGo/releases):
`steggo` (CLI), `steggo-tui` (TUI), `steggo-gui` (GUI).

### Option 2: Build from Source (Recommended)

```bash
# Requires Go 1.26+
git clone https://github.com/yikeqiyiguo/StegGo.git
cd StegGo

# Windows / Linux / macOS
go build -o steggo ./cmd/cli
go build -o steggo-tui ./cmd/tui

# GUI (Fyne requires cgo + a C compiler)
#   Windows: install MinGW-w64 (scoop install mingw)
#   Linux:   apt install gcc libgl1-mesa-dev xorg-dev
cd cmd/gui && CGO_ENABLED=1 go build -o ../../steggo-gui .
```

### Option 3: One-Click Scripts

```powershell
# Windows
.\build.ps1            # CLI + TUI
.\build.ps1 -Gui       # also build GUI (requires gcc)
.\build.ps1 -Test      # build and test
```

```bash
# Linux / macOS
make build tui         # build CLI + TUI
make gui               # build GUI (requires cgo dependencies)
make cross             # cross-compile dist/ (linux/darwin/windows × amd64/arm64)
make test              # run tests
```

### Option 4: Pluggable Docker Image

```bash
# TARGET options: cli | tui | all (default) | gui
docker build --build-arg TARGET=all   -t steggo:2.0.0 .
docker build --build-arg TARGET=gui   -t steggo-gui:2.0.0 .   # includes Linux desktop GUI

# Multi-architecture (buildx)
docker buildx build --platform linux/amd64,linux/arm64 -t steggo:2.0.0 .

# Run (no GUI inside the container — suitable for batch/service use)
docker run --rm steggo:2.0.0 version
docker run --rm -v "$PWD:/data" steggo:2.0.0 hide -c /data/a.png -s /data/msg.txt -p pass -o /data/a.steg.png
```

### Verify Installation

```bash
steggo version
steggo info
```

---

## Quick Start

### 1. Hide a Secret into an Image

```bash
steggo hide -c photo.png -s secret.txt -p "MyStrongPass!" -o steg.png
```

- `-c` carrier (PNG/BMP/TIFF etc., type auto-detected, lossy formats blocked)
- `-s` secret file (any format; add `--dir` to pack an entire directory)
- `-p` password (if omitted, secure interactive input with no echo)
- `-b` bits per channel 1-4 (default 1); `--mask` channel mask bit0=R bit1=G bit2=B
- `-a` algorithm: `lsb|dct|dwt|hugo|wow|uniward` (default lsb)

### 2. Extract a Secret (Auto-Scan Algorithm + V1 Compatible)

```bash
steggo extract -c steg.png -p "MyStrongPass!" -o ./output/
```

### 3. Digital Watermark (Publicly Extractable)

```bash
steggo watermark embed -c photo.png -m "© 2026 YourName" -o wm.png
steggo watermark extract -c wm.png     # prints: © 2026 YourName
```

### 4. Self-Audit (Detect Whether an Image Carries Hidden Data)

```bash
steggo audit -i photo.png
```

### 5. Nested / Russian-Doll Steganography

```bash
# Three layers nested: inner -> outer (a.png <- b.png <- c.png)
steggo nested embed -c a.png,b.png,c.png -s secret.txt -p pass -o ./
steggo nested extract -c c.png -d 3 -p pass -o ./
```

### 6. Integrity Verification

```bash
steggo verify -f steg.png
```

---

## Command Reference

### `hide` — Embed a Secret

```
steggo hide -c <carrier> -s <secret> [-o <output>] [-p <password>] [-a lsb] [-b 1] [--mask 7] [--dir] [--keyfile <file>] [--machine] [--fake-file <decoy> --fake-pass <fake-password>]
```

| Flag | Description |
|------|-------------|
| `-a, --algorithm` | Image algorithm `lsb\|dct\|dwt\|hugo\|wow\|uniward` (default lsb) |
| `-b, --bits` | LSB bits per channel 1-4 (default 1) |
| `--mask` | Channel mask bit0=R bit1=G bit2=B (default all on) |
| `--quality` | DCT quantization step 1-32 |
| `--levels` | DWT decomposition levels 1-3 |
| `--cost` | Adaptive cost function `hill\|wow\|uniward` |
| `--keyfile` / `--machine` | Three-factor: key file / bind to machine fingerprint |
| `--fake-file` / `--fake-pass` | Deniable: decoy file and fake password |
| `--dir` / `--name` | Directory-pack embedding / custom output filename |

### `extract` — Extract a Secret

```
steggo extract -c <carrier> [-o <dir>] [-p <password>] [--keyfile <file>] [--machine] [--algorithm <algo>]
```

When `--algorithm` is omitted, the algorithm matrix is auto-scanned; V1 legacy format falls back automatically.

### `watermark` — Digital Watermark

```
steggo watermark embed -c <image> -m <watermark> [-o <output>]
steggo watermark extract -c <image>
```

Watermarks are unencrypted and publicly extractable — suitable for copyright attribution.

### `nested` — Nested Steganography

```
steggo nested embed -c <carrier list (inner->outer, comma-separated)> -s <secret> [-p <password>] [-o <dir>]
steggo nested extract -c <outermost carrier> -d <layers> [-p <password>] [-o <dir>]
```

### `audit` — Steganalysis Self-Audit

```
steggo audit -i <image> [--json]
```

Outputs chi-square test P-value, RS-analysis embedding rate, SPA skewness, and an overall verdict.

### `capacity` — Capacity Estimation

```
steggo capacity -i <carrier> [-b <bit depth>] [--json]
```

Without `-b`, prints the capacity matrix for bit depths 1-4.

### `quality` — Quality Assessment

```
steggo quality --orig <original image> --steg <steg image> [--json]
```

Outputs PSNR / SSIM.

### `batch` — Batch Operations

```
steggo batch embed -d <dir> -s <secret> [-o <dir>] [-p <password>] [-b 2] [--recursive] [--concurrency 4]
steggo batch extract -d <dir> [-o <dir>] [-p <password>] [--recursive] [--concurrency 4]
```

### `shamir` — Threshold Secret Sharing

```
steggo shamir split -i <secret file> -n <total shares> -k <threshold> [-o <dir>]
steggo shamir recover -d <shares dir> -k <threshold> -o <output file>
```

Any k of n shares recover the secret; fewer than k shares reveal nothing.

### `zerowidth` — Zero-Width Character Steganography

```
steggo zerowidth hide -c <text carrier> -s <secret> [-o <output>] [-p <password>]
steggo zerowidth extract -i <text carrier> [-o <dir>] [-p <password>]
```

### `verify` — Integrity Verification

```
steggo verify -f <file> [-h <SHA256>]
```

### `info` / `version`

```
steggo info       # environment & support information
steggo version    # version number
```

> All commands support the `-q` silent mode.

---

## Design Overview

```
Secret file ──ZIP──> Plaintext payload ──three-factor PBKDF2 key derivation──> AES-256-GCM encryption
                                                                        │
                                                              SHA256 digest binding (tamper-proof)
                                                                        │
       Carrier dispatch: LSB/DCT/DWT/HUGO/WOW/UNIWARD / trailing container / zero-width / video sharding
```

| Security Property | Implementation |
|-------------------|----------------|
| Key derivation | PBKDF2-SHA256, **210,000** iterations; combinable three factors: password + key file + machine fingerprint |
| Encryption | AES-256-GCM (authenticated encryption — protects against both theft and tampering) |
| Randomness | Fresh `crypto/rand` salt + nonce on every embed; coordinate seed is a fixed constant, the password only participates in payload encryption/decryption (on a wrong password all algorithms uniformly report "wrong password" — no more "only some algorithms can extract" discrepancies) |
| Deniability | Dual-ciphertext structure: the real password unlocks the real payload, a fake password unlocks the decoy region; no way to prove the real payload exists |
| Memory safety | Keys/plaintext/intermediate state zeroed with `Wipe()` after use |
| Error handling | A wrong password only reports failure — no plaintext information is leaked |
| Format safety | Lossless-carrier whitelist + magic-number/extension dual blacklist for lossy formats |

---

## Anti-Detection Principles

| Detection Method | Countermeasure |
|------------------|----------------|
| Chi-square test | Deterministic pseudo-random walk dispersion + noise padding breaks adjacent-pixel LSB statistics |
| RS analysis | Three-channel round-robin embedding + noise padding reduces flip-rate regularity |
| SPA analysis | Avoids linear probing-style contiguous embedding; randomized coordinates + redundancy padding |
| Modern steganalysis | QIM embedding in mid-frequency DCT/DWT coefficients in high-texture regions; HUGO/WOW/UNIWARD cost-weighted rejection sampling concentrates changes in texture-complex areas |

The built-in self-audit module ships chi-square / RS / SPA detectors so you can validate carrier concealment in advance.

---

## Project Structure

```
StegGo/
├── cmd/
│   ├── cli/              # CLI (Cobra, 13 subcommands)
│   ├── tui/              # TUI (BubbleTea, includes watermark form)
│   └── gui/              # GUI (Fyne, standalone Go module, requires cgo)
├── internal/
│   ├── common/           # File IO / audit log / magic constants / secure wipe
│   ├── crypto/           # V3 payload wrapper / three-factor / deniable / zero-width / algorithm ID map
│   ├── algorithm/        # Six algorithm plugins + cost functions + chi-square/RS/PSNR/SSIM analysis
│   ├── carrier/          # Carrier interface + registry + image/trailing/zero-width + polyglot + nested
│   └── service/          # Embed/extract orchestration + scan matrix + batch + Shamir + watermark + audit reports
├── pkg/                  # V1 compatibility layer (steg/crypto/carrier/task, auto-fallback)
├── testdata/             # Test carriers and samples
├── Dockerfile            # Pluggable offline image (TARGET=cli|tui|all|gui, buildx multi-arch)
├── build.ps1             # Windows one-click build
├── Makefile              # Generic build + cross compilation + pluggable docker
└── docs/                 # SDK docs + disclaimer
```

---

## FAQ

**Q: Why are lossy formats like JPG/MP3 blocked?**  
A: Lossy compression destroys payload bits. To guarantee data recoverability, only lossless carriers are accepted; the blacklist validates both magic numbers and extensions to prevent masqueraded file headers.

**Q: How much data can an image hold?**  
A: For LSB with bit depth `b`, capacity ≈ `width × height × 3 × b / 8` bytes; DCT/DWT hold roughly 1/8–1/3 of the same size; adaptive algorithms depend on texture complexity. Run `steggo capacity` for an exact estimate.

**Q: Do I need to know the parameters to extract DCT/DWT?**  
A: No. Extraction auto-scans the algorithm/parameter matrix and succeeds on the first combination that passes integrity validation — the experience is identical to LSB.

**Q: What if I forget my password?**  
A: It cannot be recovered. AES-256-GCM has no backdoor; the password (and three-factor keys) is the only source of the key.

**Q: Will my image be detected after embedding?**  
A: Coordinate randomization + noise padding + cost-weighted embedding resist chi-square/RS/SPA and modern steganalysis. Run `steggo audit` on the carrier beforehand to verify concealment.

**Q: How do I distribute a secret among multiple people?**  
A: Use `steggo shamir split -n 5 -k 3` — any 3 of 5 shares recover the secret; 2 shares reveal nothing.

**Q: Why doesn't the GUI compile?**  
A: Fyne depends on cgo (OpenGL) on desktops. Install MinGW-w64 (Windows) or xorg-dev (Linux), then build with `CGO_ENABLED=1`.

**Q: How do I build only one artifact in Docker?**  
A: `docker build --build-arg TARGET=cli` (or `tui`/`gui`); the default `all` includes both CLI and TUI.

---

## Disclaimer

This tool is provided **for learning steganography, cryptography, and personal information-security research only**. Users must comply with all applicable laws and may only operate on carriers **they own or are explicitly authorized to use**. Any illegal use is strictly prohibited. See the [Disclaimer](docs/DISCLAIMER.md) for details.

---
