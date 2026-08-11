<div align="center">

# 🔐 StegGo

**AES-256 加密隐写术工具 · 把秘密藏进图片、音频和 PDF**

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)]()
[![Build](https://img.shields.io/badge/Build-Passing-brightgreen)]()

*Hide any file / password / private note inside a photo, audio file, or PDF.*  
*Only StegGo + your password can recover it — the carrier looks completely untouched.*

[快速开始](#快速开始) · [命令参考](#命令参考) · [安全设计](#安全设计) · [支持格式](#支持格式) · [贡献](#贡献)

</div>

---

## 目录

- [功能特性](#功能特性)
- [支持格式](#支持格式)
- [安装](#安装)
- [快速开始](#快速开始)
- [命令参考](#命令参考)
- [工作流程示例](#工作流程示例)
- [安全设计](#安全设计)
- [项目结构](#项目结构)
- [常见问题](#常见问题)
- [贡献](#贡献)
- [许可证](#许可证)
- [免责声明](#免责声明)

---

## 功能特性

| 特性 | 说明 |
|------|------|
| 🔒 **AES-256-GCM 加密** | PBKDF2-SHA256 派生密钥（10 万轮），每次使用随机 salt + nonce |
| 🖼️ **PNG 真隐写（LSB）** | 修改每像素 RGB 最低位，人眼完全不可见，±1 色阶差异 |
| 📎 **EOF 追加隐写** | JPEG / 音频 / PDF 文件末尾追加加密载荷，查看器/播放器完全忽略 |
| ✅ **哈希完整性校验** | 嵌入后自动生成 `.sha256` 侧车文件，随时验证载体是否被篡改 |
| 📦 **批量操作** | 一条命令批量嵌入/提取整个目录的所有受支持文件 |
| 🚫 **错误密码强拒绝** | GCM 认证标签校验失败立即报错，不返回任何乱码明文 |

---

## 支持格式

| 类型 | 格式 | 隐写方式 |
|------|------|---------|
| 图片 | `.png` | **LSB 像素隐写** — 修改最低有效位，文件结构合法 |
| 图片 | `.jpg` / `.jpeg` | **EOF 追加** — JPEG 解码器忽略 `FFD9` 之后的字节 |
| 音频 | `.wav` `.mp3` `.flac` `.ogg` `.aac` `.m4a` | **EOF 追加** — 播放器只读取帧数据 |
| 文档 | `.pdf` | **EOF 追加** — PDF 阅读器只读到 `%%EOF` 标记 |

> **容量上限**：PNG LSB 方式可藏 `(像素数 × 3) / 8` 字节；EOF 追加方式理论无上限。

---

## 安装

### 方式一：从源码构建（推荐）

```bash
# 需要 Go 1.21+
git clone https://github.com/your-username/StegGo.git
cd StegGo
go build -o steggo ./cmd/steggo

# Windows
go build -o steggo.exe ./cmd/steggo
```

### 方式二：go install

```bash
go install github.com/your-username/StegGo/cmd/steggo@latest
```

### 方式三：下载预编译二进制

前往 [Releases](https://github.com/your-username/StegGo/releases) 下载对应平台的可执行文件，无需安装 Go。

### 验证安装

```bash
steggo --help
```

---

## 快速开始

### 1. 准备一张载体图片和一个秘密文件

```bash
# 秘密文件可以是任意格式：txt / pdf / zip / 私钥文件 等
echo "password=hunter2, API_KEY=ABCD-1234" > secret.txt
```

### 2. 嵌入（藏匿）

```bash
steggo embed -c photo.png -s secret.txt -p "MyStrongPass!" -o steg.png
```

输出：
```
[OK] Embedded  : secret.txt -> steg.png
[OK] Steg hash : a3f9c2...  -> steg.png.sha256
```

### 3. 提取（恢复）

```bash
steggo extract -c steg.png -p "MyStrongPass!" -o ./output/
```

输出：
```
[OK] Extracted : secret.txt -> output/secret.txt
```

### 4. 验证完整性

```bash
steggo verify -c steg.png -H steg.png.sha256
```

输出：
```
[OK]   Hash match : steg.png is intact
```

---

## 命令参考

### `embed` — 嵌入秘密文件

```
steggo embed -c <载体文件> -s <秘密文件> -p <密码> -o <输出文件>
```

| 参数 | 简写 | 说明 |
|------|------|------|
| `--carrier` | `-c` | 载体文件路径（PNG / JPEG / 音频 / PDF） |
| `--secret` | `-s` | 要隐藏的秘密文件路径（任意格式） |
| `--password` | `-p` | 加密密码（越长越安全） |
| `--output` | `-o` | 输出的隐写文件路径 |

**示例：**
```bash
# 藏进 PNG（LSB 隐写）
steggo embed -c photo.png -s keys.txt -p "Pass123!" -o steg.png

# 藏进 MP3（EOF 追加）
steggo embed -c music.mp3 -s note.txt -p "Pass123!" -o out.mp3

# 藏进 PDF（EOF 追加）
steggo embed -c document.pdf -s secret.zip -p "Pass123!" -o out.pdf
```

---

### `extract` — 提取秘密文件

```
steggo extract -c <隐写文件> -p <密码> -o <输出目录>
```

| 参数 | 简写 | 说明 |
|------|------|------|
| `--carrier` | `-c` | 包含隐藏数据的隐写载体文件 |
| `--password` | `-p` | 解密密码 |
| `--output` | `-o` | 提取文件的输出目录 |

**示例：**
```bash
steggo extract -c steg.png -p "Pass123!" -o ./recovered/
```

> ⚠️ 密码错误时会立即报错：`decryption failed: wrong password or corrupted data`

---

### `verify` — 哈希完整性校验

```
steggo verify -c <文件> [-H <sha256文件>]
```

| 参数 | 简写 | 说明 |
|------|------|------|
| `--carrier` | `-c` | 要验证的文件 |
| `--hash-file` | `-H` | `.sha256` 侧车文件（默认：`<文件>.sha256`） |

**示例：**
```bash
steggo verify -c steg.png                        # 自动找 steg.png.sha256
steggo verify -c steg.png -H my_hash.sha256      # 指定 hash 文件
```

---

### `batch-embed` — 批量嵌入

```
steggo batch-embed -d <载体目录> -s <秘密文件> -p <密码> -o <输出目录>
```

将同一个秘密文件嵌入目录中所有受支持的文件：

```bash
steggo batch-embed -d ./photos/ -s secret.txt -p "Pass123!" -o ./steg_photos/
```

---

### `batch-extract` — 批量提取

```
steggo batch-extract -d <隐写目录> -p <密码> -o <输出目录>
```

从目录中所有隐写文件批量提取：

```bash
steggo batch-extract -d ./steg_photos/ -p "Pass123!" -o ./recovered/
```

---

## 工作流程示例

### 场景：把 SSH 私钥藏进风景照片

```bash
# 嵌入私钥
steggo embed \
  -c vacation.png \
  -s ~/.ssh/id_rsa \
  -p "VacationKey2026!" \
  -o vacation_steg.png

# 正常分享 vacation_steg.png，任何人打开都是普通照片
# 需要时恢复私钥
steggo extract \
  -c vacation_steg.png \
  -p "VacationKey2026!" \
  -o ~/.ssh/
```

### 场景：批量为相册里的每张图片嵌入同一份密码本

```bash
steggo batch-embed -d ./album/ -s passwords.txt -p "AlbumPass!" -o ./album_steg/
# 之后批量提取
steggo batch-extract -d ./album_steg/ -p "AlbumPass!" -o ./secrets/
```

---

## 安全设计

```
载体文件
└── 隐写层（PNG LSB 或 EOF 追加）
    └── 载荷结构（二进制）
        ┌─────────────────────────────────────────────────┐
        │ MAGIC[8B]  name_len[2B]  filename  data_len[4B] │
        │                     AES-256-GCM 密文             │
        └─────────────────────────────────────────────────┘
                              │
                    ┌─────────────────────┐
                    │   AES-256-GCM 加密   │
                    │                     │
                    │  PBKDF2-SHA256 派生  │
                    │  salt   : 16B 随机   │
                    │  nonce  : 12B 随机   │
                    │  rounds : 100,000   │
                    │  GCM 认证标签: 16B   │
                    └─────────────────────┘
```

| 安全属性 | 实现 |
|---------|------|
| 密钥强度 | AES-256（256-bit 密钥） |
| 密钥派生 | PBKDF2-SHA256，100,000 次迭代，16 字节随机 salt |
| 加密模式 | GCM（Galois/Counter Mode）— 同时提供加密和认证 |
| 随机性 | 每次 embed 使用 `crypto/rand` 生成全新 salt + nonce |
| 防重放 | 相同内容 + 相同密码，每次输出结果完全不同 |
| 防篡改 | GCM 认证标签 + `.sha256` 完整性校验双重保护 |
| 错误信息 | 密码错误只返回 "wrong password"，不泄露任何明文信息 |

> ⚠️ **注意**：EOF 追加方式（JPEG/音频/PDF）对专业隐写分析工具（stegdetect）可见。如需高隐蔽性，请使用 PNG LSB 方式。

---

## 项目结构

```
StegGo/
├── cmd/
│   └── steggo/
│       └── main.go               # CLI 入口（Cobra 框架）
├── internal/
│   ├── crypto/
│   │   └── aes.go                # AES-256-GCM 加解密 + PBKDF2 密钥派生
│   ├── hash/
│   │   └── hash.go               # SHA-256 文件哈希校验
│   └── steganography/
│       ├── payload.go            # 载荷二进制编解码
│       ├── image.go              # PNG LSB 隐写 + JPEG EOF 追加
│       ├── audio.go              # WAV/MP3/FLAC/OGG/AAC/M4A EOF 追加
│       ├── pdf.go                # PDF EOF 追加
│       └── steg.go               # Embed / Extract / Batch 高层调度
├── testdata/
│   └── carrier.png               # 测试用示例载体图片
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

---

## 常见问题

**Q：嵌入后图片文件大小会变化吗？**  
A：PNG LSB 方式文件大小几乎不变（像素数据量相同，重新编码后可能略有差异）；EOF 追加方式文件大小 = 原文件 + 载荷大小。

**Q：PNG 图片能藏多大的文件？**  
A：理论容量 = `像素总数 × 3 / 8` 字节。一张 1920×1080 的图可藏约 **760 KB**。

**Q：能同时藏多个文件吗？**  
A：当前版本每次 embed 只藏一个文件。可以先将多个文件打包成 `.zip` 再嵌入。

**Q：忘记密码了怎么办？**  
A：无法恢复。AES-256-GCM 没有后门，密码是唯一解密手段。

**Q：提取出来的文件名是什么？**  
A：自动使用嵌入时秘密文件的原始文件名。

**Q：Windows / macOS / Linux 都支持吗？**  
A：支持，Go 跨平台编译，行为一致。

---

## 贡献

欢迎提交 Issue 和 Pull Request！

```bash
# Fork 并克隆
git clone https://github.com/your-username/StegGo.git
cd StegGo

# 创建特性分支
git checkout -b feature/your-feature

# 运行测试
go test ./...

# 提交
git commit -m "feat: your feature description"
git push origin feature/your-feature
```

**Roadmap / 欢迎贡献的方向：**
- [ ] WAV LSB 真隐写（DCT 系数替换）
- [ ] 多文件同时嵌入
- [ ] GUI 界面（Fyne / Wails）
- [ ] 进度条显示（批量操作时）
- [ ] 单元测试覆盖率提升
- [ ] `go install` 支持的正式 Release

---

## 许可证

[MIT License](LICENSE) © 2026

---

## 免责声明

本工具仅供学习隐写术、密码学及个人隐私保护研究使用。  
请勿将其用于任何违法活动。使用者需自行承担法律责任。

---

<div align="center">
  <sub>Built with ❤️ in Go · <a href="https://github.com/your-username/StegGo">GitHub</a></sub>
</div>
