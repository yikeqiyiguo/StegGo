# StegGo 使用文档

> 本文档面向用户，介绍 StegGo V2.1.1 的全部功能与三端（CLI / TUI / GUI）操作方式。
> 开发者 SDK 文档请见 [SDK.md](SDK.md)。

---

## 功能概览

StegGo 是一款**完全离线**的抗检测隐写工具，支持将任意文件以加密形态藏入图片、音频、PDF、文本、视频等无损载体中。核心能力覆盖：

| 能力 | 说明 |
|------|------|
| **六算法隐写** | LSB / DCT-QIM / DWT-QIM / HUGO / WOW / UNIWARD，注册表插件化可扩展 |
| **自动扫描提取** | 无需记住算法与参数，自动遍历矩阵组合直至通过完整性校验 |
| **三因子加密** | 密码 + 密钥文件 + 本机指纹 → PBKDF2-SHA256 → AES-256-GCM |
| **可否认加密** | `--fake-file/--fake-pass` 双密文结构，假密码仅解开诱饵区 |
| **数字水印** | 公开可提取的版权标记（固定种子 LSB 嵌入） |
| **套娃嵌套** | 多层载体递归隐写，每层独立加密 |
| **Shamir 门限分片** | GF(2^8) (k,n) 分片，任意 k 片恢复 |
| **自检审计** | 卡方检验 / RS 分析 / SPA 分析，提前验证载体隐蔽性 |
| **容量预检** | 精确计算 1-4 位深度容量矩阵 |
| **质量评估** | PSNR / SSIM 对比原图与隐写图 |
| **批量处理** | 并发嵌入/提取目录内全部载体 |
| **零宽字符隐写** | TXT/MD 文本载体，U+200B/U+200C 编码，肉眼不可见 |
| **有损拦截** | JPG/MP3/AAC 等有损格式魔数+扩展名双重黑名单 |

---

## 三端界面

StegGo 提供三种交互方式，功能完全一致

---

## 核心功能详解

### 1. 隐写嵌入（hide）

将秘密文件（或整个目录）嵌入载体。流程为：

```
秘密文件 → ZIP 压缩 → AES-256-GCM 加密 → 载荷封装(V3) → 算法嵌入 → 输出载体
```

**六算法说明：**

| 算法 | 原理 | 适用场景 | 可调参数 |
|------|------|---------|---------|
| **LSB** | 最低有效位替换（1-4 位/通道） | 通用、高容量 | 位深度 1-4、通道掩码 |
| **DCT-QIM** | 离散余弦变换 + 量化索引调制 | 抗 JPEG 再压缩 | 量化步长 1-32、块大小 |
| **DWT-QIM** | 离散小波变换 + 量化索引调制 | 多分辨率嵌入 | 分解级数 1-3 |
| **HUGO** | 高维通用最陡下降成本函数 | 高安全场景 | 高斯核标准差 |
| **WOW** | 小波方向性成本加权 | 纹理丰富图像 | 成本函数阈值 |
| **UNIWARD** | 通用小波域成本加权 | 通用高隐蔽 | 权重参数 |

**三因子加密：** 默认仅密码即可使用；如需更高安全：
- `--keyfile <文件>`：密钥文件作为第二因子
- `--machine`：绑定本机硬件指纹作为第三因子
- 三因子可任意组合，全部丢失则无法恢复

**可否认加密（可选）：**
- `--fake-file <诱饵文件>`：嵌入一份假载荷
- `--fake-pass <假密码>`：用假密码提取时只得到诱饵
- 真实密码得到真实载荷，外部无法证明真实载荷存在

---

### 2. 自动扫描提取（extract）

提取时**无需指定算法与参数**。StegGo 内部维护扫描矩阵：

```
LSB: 位深度 2/1/3/4 + 三通道掩码
DCT: 质量因子 8/16
DWT: 级数 2/1/3
HUGO / WOW / UNIWARD: 默认参数
```

对每种组合尝试提取并校验载荷完整性（SHA256），首个通过的组合即输出。
若全部 V2 组合失败，自动回退 V1 兼容模式（旧格式 STEGGO01）。

---

### 3. 数字水印（watermark）

公开可提取的版权标记，**无需密码**：
- 嵌入：固定种子派生坐标，LSB depth=1 写入水印文本
- 提取：任何人用 `watermark extract` 即可读取

适合在图片中声明版权归属，不影响正常使用（视觉差异极小）。

---

### 4. 套娃嵌套（nested）

多层载体递归隐写，每层独立加密：
- 嵌入：`steggo nested embed -c a.png,b.png,c.png -s secret.txt -p pass`
  - 先嵌入 a.png（最内层），再嵌入 b.png，再嵌入 c.png（最外层）
- 提取：`steggo nested extract -c c.png -d 3 -p pass`
  - 从 c.png 逐层剥开，共 3 层

---

### 5. 自检审计（audit）

对图片运行三种经典隐写分析器，提前评估载体隐蔽性：

| 检测器 | 原理 | 判定阈值 |
|--------|------|---------|
| 卡方检验 | 相邻像素 LSB 分布均匀性 | P 值 < 0.05 提示异常 |
| RS 分析 | 翻转组与正则组的统计偏差 | 嵌入率 > 0.03 提示异常 |
| SPA 分析 | 样本对分析估计嵌入率 | 偏斜度 > 阈值提示异常 |

V2 审计引擎采用双指标综合判定：
- **CLEAN**：未检测到隐写痕迹
- **SUSPICIOUS**：卡方异常 且 RS 高嵌入率，风险较高
- **LOW TEXTURE**：低纹理图片（卡方可能误报），风险较低

---

### 6. 容量预检（capacity）

精确计算载体可容纳字节数：
- LSB 位深度 b 下：`宽 × 高 × 3 × b / 8` 字节（含 77 字节头部开销）
- DCT/DWT 约为同尺寸 1/8 ~ 1/3
- HUGO/WOW/UNIWARD 受纹理复杂度影响

支持输出 1-4 位深度容量矩阵，帮助选择最佳位深。

---

### 7. 质量评估（quality）

对比原图与隐写图：
- **PSNR**（峰值信噪比）：≥ 35 dB 优秀，≥ 30 dB 良好
- **SSIM**（结构相似度）：≥ 0.98 优秀，≥ 0.90 良好

LSB 位深度越低、DCT 量化步长越小，质量越高；自适应算法自动将改动集中于纹理区，整体质量保持较好。

---

### 8. 批量处理（batch）

对目录内全部支持载体执行嵌入或提取：
- 默认扩展名：`.png` `.bmp` `.tif` `.tiff` `.wav` `.flac` `.pdf` `.mp4` `.mkv` `.txt` `.md`
- 支持 `--recursive` 递归子目录
- 可指定 `--concurrency` 并发数

---

### 9. Shamir 门限分片（shamir）

GF(2^8) 有限域 (k,n) 分片：
- `split`：将秘密拆成 n 片，任意 k 片可完整恢复
- `recover`：凑齐 ≥ k 片即可恢复，< k 片得不到任何信息

典型用途：将 n 片分发给 n 个人/载体，需 k 人同意才能恢复秘密。

---

### 10. 零宽字符隐写（zerowidth）

在 TXT/MD 文本末尾以 U+200B（零宽空格）和 U+200C（零宽非连接符）编码秘密：
- 肉眼完全不可见，文本渲染零感知
- 数据仍走完整加密链路（ZIP + AES-256-GCM）

---

### 11. 载体安全拦截

有损格式会破坏载荷位，StegGo 在载体层即拦截：

| 黑名单 | 检测方式 |
|--------|---------|
| JPG/JPEG | 魔数 `FF D8 FF` + 扩展名 |
| MP3 | 魔数 + 扩展名 |
| AAC/OGG/M4A/WMA | 扩展名 |
| FLAC | 误标拦截（注：FLAC 为无损格式，实际已支持） |

载体白名单：PNG、BMP、TIFF、WAV、FLAC、PDF、TXT、MD、MP4、MKV、WEBM。

---

## CLI 命令速查

### 嵌入秘密

```bash
steggo hide -c photo.png -s secret.txt -p "pass" -o steg.png
```

常用参数：
- `-a lsb|dct|dwt|hugo|wow|uniward` — 算法（默认 lsb）
- `-b 1-4` — LSB 位深度（默认 1）
- `--mask 7` — 通道掩码 bit0=R bit1=G bit2=B（默认 7=全开）
- `--quality 8` — DCT 量化步长
- `--levels 2` — DWT 分解级数
- `--keyfile key.bin --machine` — 三因子
- `--fake-file fake.txt --fake-pass "fake"` — 可否认
- `--dir` — 打包目录嵌入
- `--name custom.zip` — 自定义提取文件名

### 提取秘密

```bash
steggo extract -c steg.png -p "pass" -o ./out/
```

- 未指定 `-a` 时自动扫描算法矩阵
- V1 旧格式自动回退兼容

### 水印

```bash
steggo watermark embed -c photo.png -m "Copyright 2026" -o wm.png
steggo watermark extract -c wm.png
```

### 套娃

```bash
steggo nested embed -c inner.png,middle.png,outer.png -s secret.txt -p pass -o ./
steggo nested extract -c outer.png -d 3 -p pass -o ./
```

### 审计

```bash
steggo audit -i photo.png           # 人类可读
steggo audit -i photo.png --json    # JSON 输出
```

### 容量

```bash
steggo capacity -i photo.png        # 1-4 位矩阵
steggo capacity -i photo.png -b 2   # 仅查 2 位
```

### 质量

```bash
steggo quality --orig photo.png --steg steg.png --json
```

### 批量

```bash
steggo batch embed -d ./carriers/ -s secret.txt -o ./out/ -p pass -b 2 --recursive --concurrency 4
steggo batch extract -d ./carriers/ -o ./out/ -p pass --recursive --concurrency 4
```

### Shamir

```bash
steggo shamir split -i secret.txt -n 5 -k 3 -o ./shares/
steggo shamir recover -d ./shares/ -k 3 -o recovered.txt
```

### 零宽字符

```bash
steggo zerowidth hide -c cover.txt -s secret.txt -p pass -o steg.txt
steggo zerowidth extract -i steg.txt -p pass -o ./out/
```

### 其他

```bash
steggo verify -f steg.png           # 完整性校验
steggo info                         # 环境信息
steggo version                      # 版本号
```

> 所有命令支持 `-q` 静默模式。

---

## TUI 操作指南

启动：`./steggo-tui`

```
StegGo V2.0 TUI

[1] 嵌入秘密      [5] 批量嵌入
[2] 提取秘密      [6] 批量提取
[3] 数字水印      [7] Shamir 分片
[4] 自检审计      [8] 零宽字符隐写

[0] 退出
```

操作方式：
1. 键盘输入数字选择功能
2. 按提示填写表单（载体路径、秘密路径、密码等）
3. 密码字段支持安全输入（不回显）
4. 确认后自动执行，输出结果

TUI 直接调用 `internal/service` 业务层，功能与 CLI 完全一致。

---

## GUI 操作指南

启动：`./steggo-gui.exe`（Windows）或 `./steggo-gui`（Linux/macOS）

界面布局（8 个标签页）：

| 标签页 | 功能 |
|--------|------|
| **嵌入** | 选择载体/秘密/输出，设置密码、算法（下拉选择 6 种）、位深度 1-4 |
| **提取** | 选择隐写载体，输入密码，自动扫描算法并展示结果 |
| **水印** | 嵌入/提取版权标记，无需密码 |
| **容量** | 选择图片，查看 1-4 位深度容量矩阵 |
| **质量** | 选择原图与隐写图，查看 PSNR/SSIM |
| **自检审计** | 对图片运行卡方+RS 双指标审计 |
| **批量** | 选择目录，批量嵌入或提取（支持递归） |
| **关于** | 版本信息与项目说明 |

操作方式：
1. 点击"浏览"按钮选择文件/目录
2. 在表单中填写参数
3. 点击"运行"按钮执行
4. 状态栏实时显示进度与结果

> GUI 通过 `pkg/steg/sdk`（V2.0 公开 SDK）驱动，与 CLI/TUI 共用同一业务层，功能完全统一。

---

## 安全建议

1. **密码强度**：使用 12 位以上混合字符（大小写+数字+符号），避免字典词汇
2. **三因子组合**：高敏感数据建议同时使用密码+密钥文件+本机绑定
3. **密钥文件保管**：密钥文件与隐写载体分开存放，建议离线存储
4. **载体选择**：优先使用高分辨率、纹理丰富的图片，容量更大、隐蔽性更好
5. **事后审计**：嵌入后运行 `audit` 验证，确保未触发检测器阈值
6. **内存安全**：密码/密钥在内存中使用后会被 `Wipe()` 清零，但操作系统仍可能交换到磁盘
7. **离线原则**：StegGo 无任何网络调用，但请确保操作环境可信

---

