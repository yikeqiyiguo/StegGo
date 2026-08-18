# StegGo 使用文档

> 本文档面向用户，介绍 StegGo V2.2.0 的全部功能与四端（CLI / TUI / GUI / WASM）操作方式。
> 开发者 SDK 文档请见 [SDK.md](SDK.md)。

---

## 功能概览

StegGo 是一款**完全离线**的抗检测隐写工具，支持将任意文件以加密形态藏入图片、音频、PDF、文本、视频等无损载体中。核心能力覆盖：

| 能力 | 说明 |
|------|------|
| **七算法隐写** | LSB / DCT-QIM / DWT-QIM / HUGO / WOW / UNIWARD / 锚定（anchored，抗旋转/裁剪/JPEG 重压缩），注册表插件化可扩展 |
| **自动扫描提取** | 无需记住算法与参数，自动遍历矩阵组合直至通过完整性校验 |
| **四因子加密** | 密码 + 密钥文件 + 本机指纹 + USB 密钥盘 → PBKDF2-SHA256 → AES-256-GCM / SM4-GCM |
| **后量子加密** | ML-KEM-768（Kyber 标准，FIPS 203，标准库零依赖）：随机 AES 主密钥经公钥封装，防量子计算破解 |
| **RS-ECC 容错** | RS(255,239) Reed-Solomon 前向纠错（low/medium/high 三档），抗社交压缩与载体局部损坏 |
| **SM4 国密** | GB/T 32907-2016 纯 Go 实现，商用密码合规场景 `--sm4` 一键切换，布局与 AES 完全兼容 |
| **可否认加密** | `--fake-file/--fake-pass` 双密文结构，假密码仅解开诱饵区 |
| **插件框架** | 统一注册中心（7 类 24 个内置插件），`plugin` 命令发现与校验，第三方可扩展 |
| **数字水印** | 公开可提取的版权标记（固定种子 LSB 嵌入） |
| **套娃嵌套** | 多层载体递归隐写，每层独立加密；`expand` 一键自动展开全部层级 |
| **预设模板** | `--preset secrecy/balance/quality` 一键切换算法与位深参数 |
| **.sg 独立容器** | 载体整体 AES/SM4 加密打包（魔数 `STEGGO4C`），独立分发 |
| **审计台账** | 操作日志 SHA256 哈希链防篡改，`ledger export` 导出 PDF 台账 |
| **批量任务清单** | TXT/CSV 清单 `task run` 批量执行隐写/提取 |
| **定时调度** | `schedule cron` 生成 crontab 定时自动解密备份 |
| **WASM 离线审计** | 浏览器纯前端只读审计（扫描/解析/元数据），数据不出本机 |
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

**七算法说明：**

| 算法 | 原理 | 适用场景 | 可调参数 |
|------|------|---------|---------|
| **LSB** | 最低有效位替换（1-4 位/通道） | 通用、高容量 | 位深度 1-4、通道掩码 |
| **DCT-QIM** | 离散余弦变换 + 量化索引调制 | 抗 JPEG 再压缩 | 量化步长 1-32、块大小 |
| **DWT-QIM** | 离散小波变换 + 量化索引调制 | 多分辨率嵌入 | 分解级数 1-3 |
| **HUGO** | 高维通用最陡下降成本函数 | 高安全场景 | 高斯核标准差 |
| **WOW** | 小波方向性成本加权 | 纹理丰富图像 | 成本函数阈值 |
| **UNIWARD** | 通用小波域成本加权 | 通用高隐蔽 | 权重参数 |
| **锚定(anchored)** | FAST 角点锚定 + Haar 小波 QIM（真实 JPEG q75 编解码模拟精化 + 最终校验） | 社交分享：抗旋转/裁剪/JPEG 重压缩，q75 重压缩 100% 可恢复 | 同步扫描邻域、锚点数量下限 |

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
anchored: 特征点锚定（自动扫描同步区 + 四方向旋转兜底）
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
- 一键展开：`steggo nested expand -c c.png -p pass -o ./expanded`
  - 自动探测嵌套深度，每层导出到 `layer_01/`、`layer_02/`…
  - 任一层不再是可识别载体即视为最内层，无需手动指定层数

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

### 12. SM4 国密算法（sm4 / --sm4）

GB/T 32907-2016 国密分组算法（纯 Go，无外部依赖）：
- 128 位分组 / 128 位密钥 / 32 轮迭代，GCM 认证加密模式
- 载荷布局与 AES-256-GCM 完全一致：`[salt 16][nonce 12][ciphertext+tag]`
- 旧载荷魔数不变，V3 载荷 flags bit7 标记 `flagSM4`，新老版本自动识别

```bash
steggo hide -c photo.png -s secret.txt --sm4 -p pass -o steg.png
steggo extract -c steg.png -p pass            # 自动识别 SM4 载荷
```

### 13. USB 硬件密钥绑定（--usb）

USB 密钥盘解锁 = 令牌文件 + 设备序列号双重绑定：
- 令牌内容与 USB 设备序列号 → SHA256 指纹（32 字节）
- 令牌被复制到其他设备即失效（设备序列号不匹配）
- 与密码 / 密钥文件 / 本机指纹组合为四因子

```bash
# 在 USB 盘（如 E:\usbkey）放置令牌后：
steggo hide -c photo.png -s secret.txt --usb E:\usbkey -p pass -o steg.png
steggo extract -c steg.png --usb E:\usbkey -p pass
```

> `usb` 子命令提供令牌生成/绑定管理：`steggo usb init <目录>`

### 14. .sg 独立容器（sg）

载体级加密打包：将任意载体整体加密为独立 `.sg` 容器（新魔数 `STEGGO4C`）：

```bash
steggo sg create -i photo.png -o photo.sg -p pass          # AES-256-GCM
steggo sg create -i photo.png -o photo.sg -p pass --sm4    # SM4-GCM
steggo sg open -i photo.sg -o ./restored -p pass
```

- 解密校验失败明确报错；`sg open` 还原原始载体
- 与隐写流程解耦：可先加密分发，再离线隐写

### 15. 审计台账（ledger）

操作日志 SHA256 哈希链防篡改：
- 每条记录 `Chain = SHA256(规范化记录 + 前一条 Chain)`
- 篡改任意历史记录 → 后续链条全部断裂 → `ledger verify` 检出
- `ledger export` 导出 PDF 台账（手写 PDF writer，中文转义，无外部依赖）

```bash
steggo ledger export -o audit.pdf
steggo ledger verify -f audit.pdf
```

### 16. 批量任务清单（task）与定时调度（schedule）

**任务清单**（TXT 键值对 / CSV 表头两种格式，支持引号包裹含空格路径）：

```bash
# task.txt
action=embed
carrier=C:\数据\载体\photo.png
secret=C:\数据\secret.txt
output=C:\数据\out
password=pass123

# task.csv
action,carrier,secret,output,password
extract,D:\backup\nested_03.png,,D:\backup\restored,pass123

steggo task run -f task.txt
steggo task run -f task.csv
```

**定时调度**（生成 Linux crontab 解密任务）：

```bash
steggo schedule cron --carrier /data/nested_03.png --output /backup --password-file /root/.steggo.pass
# 默认每日 02:30；--install 输出可直接 crontab 使用的片段
```

### 17. 算法参数预设模板（--preset）

一键切换常用参数组合：

| 预设 | 算法 | 位深 | 适用场景 |
|------|------|------|---------|
| `secrecy` | UNIWARD | 1 bit | 保密优先：抗检测最强，容量较小 |
| `balance` | DWT | 2 bit | 平衡：容量与隐蔽性折中 |
| `quality` | LSB | 1 bit | 画质优先：视觉质量最好，容量大 |

```bash
steggo hide -c photo.png -s secret.txt --preset secrecy -o steg.png
```

### 18. WASM 浏览器离线审计

`wasm/index.html` 纯前端只读审计（构建后约 3.9MB）：
- 魔数扫描：识别 V2 / V3 载荷
- 结构解析：算法 / 位深 / 尺寸 / 哈希等元数据
- 全程离线运行，数据不出本机，无后端依赖

```bash
.\build.ps1 -Wasm     # Windows
make wasm             # Linux / macOS
```

### 19. 后量子混合加密（kyber / --kyber-pub）

ML-KEM-768（Kyber 标准版，NIST FIPS 203）由 Go 标准库 `crypto/mlkem` 实现，**零外部依赖**。私钥为 64 字节种子、公钥 1184 字节，密钥文件以 0600 权限落盘。

**工作方式（混合加密）：**
1. 随机生成 AES-256 主密钥，加密载荷（配合三因子/密码派生密钥体系）
2. 主密钥经 ML-KEM-768 公钥封装（密文 1120 字节）写入载荷头
3. 提取时用私钥解封装出主密钥，再解密载荷

即使攻击者将来拥有量子计算机，也无法由封装密文反推出主密钥，从而保住 AES 会话密钥安全。

```bash
# ① 生成密钥对（接收方共享公钥，自己保管私钥）
steggo kyber keygen -o pub.kyb -k priv.kyb
steggo kyber info                     # 查看算法参数

# ② 嵌入：启用后量子混合加密
steggo hide -c photo.png -s secret.txt --kyber-pub pub.kyb -p pass -o steg.png

# ③ 提取：私钥解封装主密钥并解密
steggo extract -c steg.png --kyber-priv priv.kyb -p pass -o ./out/
```

> 说明：后量子加密与可否认（`--fake-file`）当前暂不组合使用；公钥/私钥参数与普通密码参数互斥校验。

### 20. RS-ECC 容错编码（--ecc）

RS(255,239) Reed-Solomon 前向纠错编码，将载荷按 STECC 包装后嵌入，提取时自动纠错。适用于：社交平台压缩转发、载体局部损坏、传输丢包等场景。

| 等级 | 冗余 | 每块纠错能力 | 适用场景 |
|------|------|-------------|---------|
| `low` | 低 | 最多 8 个符号 | 常规传输，开销最小 |
| `medium` | 中 | 最多 16 个符号 | 一般社交压缩 |
| `high` | 高 | 最多 64 个符号 | 重压缩/较严重损坏 |

```bash
steggo hide -c photo.png -s secret.txt --ecc high -p pass -o steg.png
steggo extract -c steg.png -p pass
# 输出示例: [+] RS-ECC 纠错: high 级 | 12 块 | 修复 3 符号 | 完好率 99.9%
```

- 提取与扫描路径自动识别 STECC 包装并纠错，无需额外参数
- 修复统计随提取结果一并输出；损坏超出纠错上限会明确报错
- 兼容旧载荷：无 STECC 魔数的载荷按原逻辑处理

### 21. 插件加载框架（plugin）

基础插件加载框架，统一登记全部可扩展能力：

```
steggo plugin                    # 按类别分组展示全部插件
steggo plugin --kind algorithm   # 仅查看隐写算法类
steggo plugin --kind kem         # 仅查看后量子 KEM 类
```

内置 25 个插件，分 7 类：**隐写算法**（lsb/dct/dwt/hugo/wow/uniward/anchored）、**载体类型**（image/audio/trailing/zerowidth/polyglot）、**对称加密**（aes-256-gcm/sm4-gcm）、**后量子 KEM**（ml-kem-768）、**容错编码**（reed-solomon）、**预设模板**（secrecy/balance/quality）、**安全工具**（three-factor/deniable/shamir/watermark/usb-key/container-sg）。

第三方开发者可在代码中调用 `plugin.Register(plugin.Info{...})` 追加新插件，注册中心为并发安全实现，供发现、校验与生态扩展。

---

## CLI 命令速查

### 嵌入秘密

```bash
steggo hide -c photo.png -s secret.txt -p "pass" -o steg.png
```

常用参数：
- `-a lsb|dct|dwt|hugo|wow|uniward|anchored` — 算法（默认 lsb）
- `-b 1-4` — LSB 位深度（默认 1）
- `--mask 7` — 通道掩码 bit0=R bit1=G bit2=B（默认 7=全开）
- `--quality 8` — DCT 量化步长
- `--levels 2` — DWT 分解级数
- `--keyfile key.bin --machine --usb E:\usbkey` — 四因子
- `--sm4` — SM4-GCM 国密算法
- `--preset secrecy|balance|quality` — 算法参数预设模板
- `--kyber-pub pub.kyb` — 后量子混合加密（ML-KEM-768，`kyber keygen` 生成）
- `--ecc low|medium|high` — RS-ECC 容错编码
- `--password-file pwd.txt` — 从文件读取密码（定时/非交互场景）
- `--fake-file fake.txt --fake-pass "fake"` — 可否认
- `--dir` — 打包目录嵌入
- `--name custom.zip` — 自定义提取文件名

### 提取秘密

```bash
steggo extract -c steg.png -p "pass" -o ./out/
steggo extract -c steg.png --kyber-priv priv.kyb -p "pass" -o ./out/
```

- 未指定 `-a` 时自动扫描算法矩阵
- V1 旧格式自动回退兼容
- `--kyber-priv` 解封装后量子主密钥；`--ecc` 载荷自动纠错并输出修复统计

### 水印

```bash
steggo watermark embed -c photo.png -m "Copyright 2026" -o wm.png
steggo watermark extract -c wm.png
```

### 套娃

```bash
steggo nested embed -c inner.png,middle.png,outer.png -s secret.txt -p pass -o ./
steggo nested extract -c outer.png -d 3 -p pass -o ./
steggo nested expand -c outer.png -p pass -o ./expanded    # 一键展开全部层级
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

### .sg 容器 / 审计台账 / 批量任务 / 调度

```bash
steggo sg create -i photo.png -o photo.sg -p pass [--sm4]
steggo sg open -i photo.sg -o ./restored -p pass
steggo ledger export -o audit.pdf
steggo ledger verify -f audit.pdf
steggo task run -f task.csv
steggo schedule cron --carrier /data/x.png --output /backup --password-file /root/.pass
```

### 后量子 / 插件 / 其他

```bash
steggo kyber keygen -o pub.kyb -k priv.kyb   # 生成 ML-KEM-768 密钥对
steggo kyber info                            # 查看后量子算法信息
steggo plugin [--kind algorithm|carrier|crypto|kem|ecc|preset|tool]  # 插件注册中心
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
| **嵌入** | 选择载体/秘密/输出，设置密码、算法（下拉选择 7 种）、位深度 1-4 |
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

