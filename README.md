<div align="center">

# 🔐 StegGo V1.0.0

**完全离线的抗检测隐写工具 · 把秘密藏进图片 / 音频 / PDF / 文本 / 视频**

[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)]()
[![CLI](https://img.shields.io/badge/CLI-✓-brightgreen)]()
[![TUI](https://img.shields.io/badge/TUI-✓-brightgreen)]()
[![GUI](https://img.shields.io/badge/GUI-Fyne-9cf)]()

*「藏得下，查不出」*

 SDK: [SDK.md](docs/SDK.md) · 免责声明: [DISCLAIMER.md](docs/DISCLAIMER.md)

</div>

---


## 核心特性

| 模块 | 说明 |
|------|------|
|  **自研抗检测 LSB** | 伪随机坐标（Xorshift64Star+PBKDF2）+ RGB 三通道轮询 + 高斯噪声填充，对抗卡方/RS/SPA 统计检测 |
|  **加密链路** | ZIP 压缩 → PBKDF2-SHA256（21 万次迭代）→ AES-256-GCM → SHA256 完整性绑定 |
|  **无损载体** | 图片（PNG/BMP/TIFF）、音频（WAV）、文档（PDF）、文本（TXT/MD 零宽字符）、视频（帧分片+XOR 冗余） |
|  **有损格式拦截** | JPG/MP3/FLAC 等有损格式一律拒绝，避免压缩破坏载荷 |
|  **敏感内存清零** | 密码、派生密钥、明文载荷使用后立即 `Wipe()` 清零 |
|  **隐写自检审计** | 卡方检验 / RS 分析 / SPA 分析，支持低纹理保护，误报率可控 |
|  **容量预检与质量评估** | `capacity` 提前查看各载体容量；`quality` 输出 PSNR/SSIM |
|  **批量与流式** | 目录批量嵌入/提取（并发 worker 池）；64MB 分块流式处理大文件 |
|  **Shamir 门限分片** | GF(2⁸) 有限域实现 (k,n) 分片，任意 k 片恢复，分片可分发到多张载体 |
|  **三端兼容** | 命令行 CLI + 终端 TUI（BubbleTea）+ 桌面 GUI（Fyne） |
|  **旧版兼容** | 兼容 V1 老格式（100k 迭代），无缝升级 |

---

## 载体支持矩阵

| 类型 | 格式 | 隐写方式 | 说明 |
|------|------|---------|------|
| 图片 | `.png` `.bmp` `.tif` `.tiff` | **抗检测 LSB** | 伪随机像素位嵌入，可调位深度 1-4 |
| 音频 | `.wav` | **尾部容器** | 不破坏 WAV 头与数据完整性 |
| 文档 | `.pdf` | **尾部容器** | 写入 `EOF` 标记之前，不破坏渲染结构 |
| 文本 | `.txt` `.md` `.markdown` | **零宽字符** | U+200B/U+200C 编码，肉眼不可见 |
| 视频 | `.mp4` `.mkv` `.webm` | **帧分片+XOR 冗余** | 分片嵌入 + 冗余纠错 |
| ~~有损~~ | `.jpg` `.jpeg` `.mp3` `.flac` | **拦截** | 有损压缩会破坏载荷，一律拒绝 |

---

## 安装

### 方式一：下载预编译二进制

前往 [Releases](https://github.com/yikeqiyiguo/StegGo/releases) 下载对应平台产物：
`steggo-<os>-<arch>`（CLI）、`steggo-tui-<os>-<arch>`（TUI）、`steggo-gui-windows-amd64.exe`（GUI）。

### 方式二：源码构建（推荐）

```bash
# 需要 Go 1.26+
git clone https://github.com/yikeqiyiguo/StegGo.git
cd StegGo

# Windows / Linux / macOS
go build -o steggo ./cmd/steggo
go build -o steggo-tui ./cmd/steggo-tui

# GUI（Fyne 需要 cgo + C 编译器）
#   Windows: 安装 MinGW-w64（scoop install mingw）
#   Linux:   apt install gcc libgl1-mesa-dev xorg-dev
cd cmd/steggo-gui && CGO_ENABLED=1 go build -o ../../steggo-gui .
```

### 方式三：一键脚本

```powershell
# Windows
.\build.ps1            # CLI + TUI
.\build.ps1 -Gui       # 追加 GUI（需 gcc）
.\build.ps1 -Test      # 构建并测试
```

```bash
# Linux / macOS
make build tui         # 构建 CLI + TUI
make gui               # 构建 GUI（需 cgo 依赖）
make test              # 运行测试
```

### 验证安装

```bash
steggo version
steggo info
```

---

## 快速开始

### 1. 嵌入秘密到图片

```bash
steggo hide -c photo.png -s secret.txt -p "MyStrongPass!" -o steg.png
```

- `-c` 载体（PNG/BMP/TIFF 等，自动识别类型）
- `-s` 秘密文件（任意格式；加 `--dir` 可打包整个目录）
- `-p` 密码（不传则安全交互输入，不回显）
- `-b` 嵌入位数 1-4，默认 2（越高容量越大，隐蔽性略降）

### 2. 提取秘密

```bash
steggo extract -c steg.png -p "MyStrongPass!" -o ./output/
```

### 3. 自检审计（检测图片是否被藏入数据）

```bash
steggo audit -i photo.png
```

### 4. 完整性校验

```bash
steggo verify -f steg.png
```

---

## 命令参考

### `hide` — 嵌入秘密

```
steggo hide -c <载体> -s <秘密> [-o <输出>] [-p <密码>] [-b 2] [--dir] [--name <名>] [--stream]
```

| 参数 | 简写 | 说明 |
|------|------|------|
| `--carrier` | `-c` | 载体文件（PNG/BMP/TIFF/WAV/PDF/TXT/MD/视频） |
| `--secret` | `-s` | 秘密文件（配合 `--dir` 打包整个目录） |
| `--output` | `-o` | 输出文件（默认 `<载体名>.steg.<原扩展名>`） |
| `--password` | `-p` | 密码（不传则交互输入） |
| `--bits` | `-b` | 图片每通道嵌入位数 1-4（默认 2） |
| `--dir` | | 目录模式：将秘密目录整体打包嵌入 |
| `--name` | | 保存的文件名（默认取秘密文件名） |
| `--stream` | | 流式处理大文件（音频/PDF） |

### `extract` — 提取秘密

```
steggo extract -c <载体> [-o <目录>] [-p <密码>] [--stream]
```

| 参数 | 简写 | 说明 |
|------|------|------|
| `--carrier` | `-c` | 隐写载体文件 |
| `--output` | `-o` | 输出目录（默认 `./extracted`） |
| `--password` | `-p` | 密码（不传则交互输入） |
| `--stream` | | 流式提取大文件 |

### `audit` — 隐写自检审计

```
steggo audit -i <图片> [--json]
```

输出卡方检验 P 值、RS 分析嵌入率、SPA 偏斜度及综合判定。

### `capacity` — 容量预检

```
steggo capacity -i <载体> [-b <位深度>] [--json]
```

无 `-b` 时输出 1-4 位深度容量矩阵。

### `quality` — 质量评估

```
steggo quality --orig <原始图> --steg <隐写图> [--json]
```

输出 PSNR / SSIM。

### `batch` — 批量操作

```
steggo batch embed -d <目录> -s <秘密> [-o <目录>] [-p <密码>] [-b 2] [--recursive] [--concurrency 4]
steggo batch extract -d <目录> [-o <目录>] [-p <密码>] [--recursive] [--concurrency 4]
```

### `shamir` — 门限分片

```
steggo shamir split -i <秘密文件> -n <总片数> -k <门限> [-o <目录>]
steggo shamir recover -d <分片目录> -k <门限> -o <输出文件>
```

任意凑齐 k 个分片即可恢复；少于 k 个得不到任何信息。

### `zerowidth` — 零宽字符隐写

```
steggo zerowidth hide -c <文本载体> -s <秘密> [-o <输出>] [-p <密码>]
steggo zerowidth extract -i <文本载体> [-o <目录>] [-p <密码>]
```

### `verify` — 完整性校验

```
steggo verify -f <文件> [-h <SHA256>]
```

### `info` / `version`

```
steggo info       # 环境与支持信息
steggo version    # 版本号
```

> 所有命令支持 `-q` 静默模式。

---

## 三端界面

| 客户端 | 技术栈 | 说明 |
|--------|--------|------|
| **CLI** `cmd/steggo` | Cobra | 全功能命令行，脚本友好，支持管道/静默输出 |
| **TUI** `cmd/steggo-tui` | BubbleTea | 菜单→表单→运行状态机，无需图形环境 |
| **GUI** `cmd/steggo-gui` | Fyne | 标签页布局（嵌入/提取/审计/批量/关于），需 cgo 构建 |

---

## 安全设计

```
秘密文件 ──ZIP──> 明文载荷 ──PBKDF2 派生密钥──> AES-256-GCM 加密
                                                    │
                                          SHA256 摘要绑定（防篡改）
                                                    │
                    载体类型分派：LSB / 尾部容器 / 零宽字符 / 视频分片
```

| 安全属性 | 实现 |
|---------|------|
| 密钥派生 | PBKDF2-SHA256，**210,000** 次迭代，16 字节随机盐 |
| 加密算法 | AES-256-GCM（认证加密，同时防窃取与防篡改） |
| 随机性 | 每次嵌入全新 `crypto/rand` 盐 + nonce；坐标种子由 PBKDF2 派生 |
| 内存安全 | 密钥/明文/中间态使用后 `Wipe()` 清零 |
| 错误处理 | 密码错误仅提示失败，不泄露任何明文信息 |
| 防暴力破解 | 21 万次 PBKDF2 迭代大幅抬高离线爆破成本 |
| 格式安全 | 仅支持无损载体；有损格式（JPG 等）在入口拦截 |

---

## 抗检测原理

| 检测手段 | 对抗策略 |
|---------|---------|
| 卡方检验 | 伪随机坐标散布 + 噪声填充，破坏相邻像素 LSB 统计规律 |
| RS 分析 | 三通道轮询嵌入 + 噪声填充，降低翻转率规律性 |
| SPA 分析 | 避免线性探测式连续嵌入，坐标随机化 + 冗余填充 |

自检审计模块同样内置以上三类检测器，可提前验证载体隐蔽性。

---

## 项目结构

```
StegGo/
├── cmd/
│   ├── steggo/          # CLI（Cobra，11 个子命令）
│   ├── steggo-tui/      # TUI（BubbleTea）
│   └── steggo-gui/      # GUI（Fyne，独立 Go Module，需 cgo）
├── pkg/
│   ├── crypto/          # AES-256-GCM / PBKDF2 / Wipe / ZIP 打包
│   ├── steg/            # 抗检测 LSB / 格式 / 审计 / 质量 / 容量 / Shamir / 流式 / 批量
│   ├── carrier/         # 载体类型识别与加载（图片/音频/PDF/文本/视频）
│   └── task/            # 并发 worker 池
├── internal/            # 早期实现（保留兼容）
├── testdata/            # 测试载体与样本
├── Dockerfile           # 完全离线部署镜像
├── build.ps1            # Windows 一键构建
├── Makefile             # 通用构建
└── docs/                # SDK 文档 + 免责声明
```

---

## 常见问题

**Q：为什么拦截 JPG？**  
A：JPEG 是有损压缩，重编码会破坏 LSB 载荷。为保证数据可恢复性，仅接受无损载体。

**Q：图片能藏多少数据？**  
A：位深度 b 下容量 ≈ `宽 × 高 × 3 × b / 8` 字节。1920×1080、b=2 时约 **1.5 MB**。可用 `steggo capacity` 精确查询。

**Q：忘记密码怎么办？**  
A：无法恢复。AES-256-GCM 无后门，密码是唯一密钥来源。

**Q：嵌入后图片会被检测出来吗？**  
A：本工具通过坐标随机化 + 噪声填充对抗卡方/RS/SPA 检测；自检前可先运行 `steggo audit` 验证。

**Q：如何将秘密分发给多人保管？**  
A：使用 `steggo shamir split -n 5 -k 3`，5 份分片任意 3 份可恢复，2 份无任何信息。

**Q：GUI 为什么编译不过？**  
A：Fyne 在桌面端依赖 cgo（OpenGL），需安装 MinGW-w64（Windows）或 xorg-dev（Linux）后以 `CGO_ENABLED=1` 构建。

---

## 免责声明

本工具仅供**学习隐写术、密码学与个人信息安全研究**使用。使用者须遵守所在地法律法规，仅可在**自己拥有或获得明确授权**的载体上进行操作。严禁用于任何非法用途。详见 [DISCLAIMER.md](docs/DISCLAIMER.md)。

---


