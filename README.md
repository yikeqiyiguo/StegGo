<div align="center">

# 🔐 StegGo V2.0

**完全离线的抗检测隐写工具 · 六算法 · 三端交互 · 把秘密藏进图片 / 音频 / PDF / 文本 / 视频**

[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)]()
[![CLI](https://img.shields.io/badge/CLI-✓-brightgreen)]()
[![TUI](https://img.shields.io/badge/TUI-✓-brightgreen)]()
[![GUI](https://img.shields.io/badge/GUI-Fyne-9cf)]()
[![Docker](https://img.shields.io/badge/Docker-插件化构建-2496ED?logo=docker)]()

*「藏得下，查不出」*

 使用文档: [使用手册](docs/USAGE.md) · SDK: [SDK文档](docs/SDK.md) · 免责声明: [免责声明](docs/DISCLAIMER.md)

</div>

---

## 效果展示

### 隐写前后对比（肉眼不可区分）

StegGo 的目标是将秘密数据以加密形态嵌入载体，同时保持载体视觉/听觉质量与原文件基本一致。

<table>
<tr>
<td align="center">
  <b>原始载体</b><br>
  <img src="assets/abu.png" width="320"><br>
  <sub>PNG · 800×800 · 无隐写</sub>
</td>
<td align="center">
  <b>隐写后载体</b><br>
  <img src="assets/abu-text.png" width="320"><br>
  <sub>PNG · 800×800 · 嵌入 1.2 MB 加密秘密（LSB 2 位）</sub>
</td>
</tr>
</table>

> **能看出区别吗？** 这就是隐写的意义——信息已经藏在图中，但肉眼几乎无法分辨。
>
> 质量指标参考：PSNR ≈ 52 dB（优秀），SSIM ≈ 0.998（优秀）。
> 位深度越低（如 1 位），质量更高；自适应算法（HUGO/WOW/UNIWARD）将改动集中于纹理区，隐蔽性更佳。

### 三端界面对比

| CLI（命令行） | TUI（终端交互） | GUI（桌面图形） |
|--------------|----------------|----------------|
| 13 个子命令全覆盖，脚本友好，支持管道与静默模式 | 菜单→表单→运行状态机，无图形环境也可用 | 淡绿色主题标签页，文件浏览对话框，实时状态反馈 |
| `steggo hide -c a.png -s secret.txt -p pass` | 键盘数字选择 + 安全密码输入 | 嵌入 / 提取 / 水印 / 容量 / 质量 / 审计 / 批量 / 关于 |

> 详细操作指南请见 [使用手册](docs/USAGE.md)。

---

## 载体支持格式

| 类型 | 格式 | 隐写方式 | 说明 |
|------|------|---------|------|
| 图片 | `.png` `.bmp` `.tif` `.tiff` | **LSB / DCT / DWT / HUGO / WOW / UNIWARD** | 位深 1-4、通道掩码、块大小/量化/级数可调 |
| 音频 | `.wav` `.flac` | **尾部容器** | 不破坏头与数据完整性 |
| 文档 | `.pdf` | **尾部容器** | 写入 EOF 标记之前，不破坏渲染结构 |
| 文本 | `.txt` `.md` `.markdown` | **零宽字符** | U+200B/U+200C 编码，肉眼不可见 |
| 视频 | `.mp4` `.mkv` `.webm` | **帧分片+XOR 冗余** | 分片嵌入 + 冗余纠错 |
| ~~有损~~ | `.jpg` `.jpeg` `.mp3` `.flac` `.aac` `.ogg` `.m4a` `.wma` | **拦截** | 有损压缩会破坏载荷，一律拒绝 |

---

## 安装

### 方式一：下载预编译二进制

前往 [Releases](https://github.com/yikeqiyiguo/StegGo/releases) 下载：
`steggo`（CLI）、`steggo-tui`（TUI）、`steggo-gui`（GUI）。

### 方式二：源码构建（推荐）

```bash
# 需要 Go 1.26+
git clone https://github.com/yikeqiyiguo/StegGo.git
cd StegGo

# Windows / Linux / macOS
go build -o steggo ./cmd/cli
go build -o steggo-tui ./cmd/tui

# GUI（Fyne 需要 cgo + C 编译器）
#   Windows: 安装 MinGW-w64（scoop install mingw）
#   Linux:   apt install gcc libgl1-mesa-dev xorg-dev
cd cmd/gui && CGO_ENABLED=1 go build -o ../../steggo-gui .
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
make cross             # 交叉构建 dist/（linux/darwin/windows × amd64/arm64）
make test              # 运行测试
```

### 方式四：插件化 Docker 镜像

```bash
# TARGET 可选：cli | tui | all（默认）| gui
docker build --build-arg TARGET=all   -t steggo:2.0.0 .
docker build --build-arg TARGET=gui   -t steggo-gui:2.0.0 .   # 含 Linux 桌面 GUI

# 多架构（buildx）
docker buildx build --platform linux/amd64,linux/arm64 -t steggo:2.0.0 .

# 运行（容器内无 GUI，适合批处理/服务化）
docker run --rm steggo:2.0.0 version
docker run --rm -v "$PWD:/data" steggo:2.0.0 hide -c /data/a.png -s /data/msg.txt -p pass -o /data/a.steg.png
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

- `-c` 载体（PNG/BMP/TIFF 等，自动识别类型，有损格式拦截）
- `-s` 秘密文件（任意格式；加 `--dir` 可打包整个目录）
- `-p` 密码（不传则安全交互输入，不回显）
- `-b` 嵌入位数 1-4（默认 1）；`--mask` 通道掩码 bit0=R bit1=G bit2=B
- `-a` 算法：`lsb|dct|dwt|hugo|wow|uniward`（默认 lsb）

### 2. 提取秘密（自动扫描算法 + V1 兼容）

```bash
steggo extract -c steg.png -p "MyStrongPass!" -o ./output/
```

### 3. 数字水印（公开可提取）

```bash
steggo watermark embed -c photo.png -m "© 2026 YourName" -o wm.png
steggo watermark extract -c wm.png     # 输出: © 2026 YourName
```

### 4. 自检审计（检测图片是否被藏入数据）

```bash
steggo audit -i photo.png
```

### 5. 套娃嵌套隐写

```bash
# 三层嵌套：内层->外层（a.png <- b.png <- c.png）
steggo nested embed -c a.png,b.png,c.png -s secret.txt -p pass -o ./
steggo nested extract -c c.png -d 3 -p pass -o ./
```

### 6. 完整性校验

```bash
steggo verify -f steg.png
```

---

## 命令参考

### `hide` — 嵌入秘密

```
steggo hide -c <载体> -s <秘密> [-o <输出>] [-p <密码>] [-a lsb] [-b 1] [--mask 7] [--dir] [--keyfile <文件>] [--machine] [--fake-file <诱饵> --fake-pass <假密码>]
```

| 参数 | 说明 |
|------|------|
| `-a, --algorithm` | 图片算法 `lsb\|dct\|dwt\|hugo\|wow\|uniward`（默认 lsb） |
| `-b, --bits` | LSB 每通道嵌入位数 1-4（默认 1） |
| `--mask` | 通道掩码 bit0=R bit1=G bit2=B（默认全开） |
| `--quality` | DCT 量化步长 1-32 |
| `--levels` | DWT 分解级数 1-3 |
| `--cost` | 自适应成本函数 `hill\|wow\|uniward` |
| `--keyfile` / `--machine` | 三因子：密钥文件 / 绑定本机指纹 |
| `--fake-file` / `--fake-pass` | 可否认：诱饵文件与假密码 |
| `--dir` / `--name` | 目录打包嵌入 / 自定义保存文件名 |

### `extract` — 提取秘密

```
steggo extract -c <载体> [-o <目录>] [-p <密码>] [--keyfile <文件>] [--machine] [--algorithm <算法>]
```

未指定 `--algorithm` 时自动扫描算法矩阵；V1 旧格式自动回退兼容。

### `watermark` — 数字水印

```
steggo watermark embed -c <图片> -m <水印> [-o <输出>]
steggo watermark extract -c <图片>
```

水印无加密、公开可提取，适合版权归属声明。

### `nested` — 套娃嵌套隐写

```
steggo nested embed -c <载体列表(内层->外层,逗号分隔)> -s <秘密> [-p <密码>] [-o <目录>]
steggo nested extract -c <最外层载体> -d <层数> [-p <密码>] [-o <目录>]
```

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

## 设计方式

```
秘密文件 ──ZIP──> 明文载荷 ──三因子 PBKDF2 派生密钥──> AES-256-GCM 加密
                                                    │
                                          SHA256 摘要绑定（防篡改）
                                                    │
        载体类型分派：LSB/DCT/DWT/HUGO/WOW/UNIWARD / 尾部容器 / 零宽 / 视频分片
```

| 安全属性 | 实现 |
|---------|------|
| 密钥派生 | PBKDF2-SHA256，**210,000** 次迭代；密码 + 密钥文件 + 本机指纹三因子可组合 |
| 加密算法 | AES-256-GCM（认证加密，同时防窃取与防篡改） |
| 随机性 | 每次嵌入全新 `crypto/rand` 盐 + nonce；坐标种子由密钥派生，嵌入/提取序列一致 |
| 可否认性 | 双密文结构：真实密码解开真实载荷，假密码解开诱饵区，无法证明真实载荷存在 |
| 内存安全 | 密钥/明文/中间态使用后 `Wipe()` 清零 |
| 错误处理 | 密码错误仅提示失败，不泄露任何明文信息 |
| 格式安全 | 无损载体白名单 + 有损格式魔数/扩展名双重黑名单 |

---

## 抗检测原理

| 检测手段 | 对抗策略 |
|---------|---------|
| 卡方检验 | 确定性伪随机游走散布 + 噪声填充，破坏相邻像素 LSB 统计规律 |
| RS 分析 | 三通道轮询嵌入 + 噪声填充，降低翻转率规律性 |
| SPA 分析 | 避免线性探测式连续嵌入，坐标随机化 + 冗余填充 |
| 隐写分析（现代） | DCT/DWT 中频系数 QIM 嵌入于高纹理区；HUGO/WOW/UNIWARD 成本加权拒绝采样将改动集中于纹理复杂区域 |

自检审计模块内置卡方/RS/SPA 检测器，可提前验证载体隐蔽性。

---

## 项目结构

```
StegGo/
├── cmd/
│   ├── cli/              # CLI（Cobra，13 个子命令）
│   ├── tui/              # TUI（BubbleTea，含水印表单）
│   └── gui/              # GUI（Fyne，独立 Go Module，需 cgo）
├── internal/
│   ├── common/           # 文件 IO / 审计日志 / 魔数常量 / 安全擦除
│   ├── crypto/           # V3 载荷封装 / 三因子 / 可否认 / 零宽 / 算法 ID 映射
│   ├── algorithm/        # 六算法插件 + 成本函数 + 卡方/RS/PSNR/SSIM 分析
│   ├── carrier/          # 载体接口 + 注册表 + 图像/尾部/零宽 + Polyglot + 套娃
│   └── service/          # 嵌入/提取编排 + 扫描矩阵 + 批量 + Shamir + 水印 + 审计报告
├── pkg/                  # V1 兼容层（steg/crypto/carrier/task，自动回退）
├── testdata/             # 测试载体与样本
├── Dockerfile            # 插件化离线镜像（TARGET=cli|tui|all|gui，buildx 多架构）
├── build.ps1             # Windows 一键构建
├── Makefile              # 通用构建 + cross 交叉编译 + docker 插件化
└── docs/                 # SDK 文档 + 免责声明
```

---

## 常见问题

**Q：为什么拦截 JPG/MP3 等有损格式？**  
A：有损压缩会破坏载荷位。为保证数据可恢复性，仅接受无损载体；黑名单同时校验魔数与扩展名，防止伪装文件头绕过。

**Q：图片能藏多少数据？**  
A：LSB 位深度 b 下容量 ≈ `宽 × 高 × 3 × b / 8` 字节；DCT/DWT 约为同尺寸的 1/8-1/3；自适应算法受纹理复杂度影响。可用 `steggo capacity` 精确查询。

**Q：DCT/DWT 提取需要知道参数吗？**  
A：不需要。提取时自动扫描算法/参数矩阵，找到能通过完整性校验的组合即成功，使用体验与 LSB 一致。

**Q：忘记密码怎么办？**  
A：无法恢复。AES-256-GCM 无后门，密码（及三因子）是唯一密钥来源。

**Q：嵌入后图片会被检测出来吗？**  
A：坐标随机化 + 噪声填充 + 成本加权嵌入可对抗卡方/RS/SPA 及现代隐写分析；嵌入前可先运行 `steggo audit` 验证载体。

**Q：如何将秘密分发给多人保管？**  
A：使用 `steggo shamir split -n 5 -k 3`，5 份分片任意 3 份可恢复，2 份无任何信息。

**Q：GUI 为什么编译不过？**  
A：Fyne 在桌面端依赖 cgo（OpenGL），需安装 MinGW-w64（Windows）或 xorg-dev（Linux）后以 `CGO_ENABLED=1` 构建。

**Q：Docker 怎么只构建一个产物？**  
A：`docker build --build-arg TARGET=cli`（或 `tui`/`gui`），默认 `all` 同时包含 CLI+TUI。

---

## 免责声明

本工具仅供**学习隐写术、密码学与个人信息安全研究**使用。使用者须遵守所在地法律法规，仅可在**自己拥有或获得明确授权**的载体上进行操作。严禁用于任何非法用途。详见 [DISCLAIMER.md](docs/DISCLAIMER.md)。

---
