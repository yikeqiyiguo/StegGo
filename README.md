# StegGo — AES-Encrypted Steganography Toolkit

**把任意文件/密码藏进图片、音频、PDF 里，外部完全看不出修改痕迹。**

---

## 特性

| 特性 | 说明 |
|------|------|
| AES-256-GCM 加密 | PBKDF2 (SHA-256, 100 000 轮) 派生密钥，GCM 认证加密 |
| PNG 真隐写 | LSB（最低有效位）修改像素 R/G/B 通道，人眼不可见 |
| JPEG/音频/PDF | EOF-append 方式，图片/播放器/阅读器完全忽略追加字节 |
| 哈希校验 | 嵌入后自动保存 `.sha256`，可随时验证载体是否被篡改 |
| 批量操作 | 一条命令批量嵌入/提取整个目录 |
| 错误密码拒绝 | GCM 认证标签不一致时立即报错，防暴力解密 |

---

## 支持格式

```
图片：  .png  (LSB 像素隐写)  |  .jpg / .jpeg  (EOF 追加)
音频：  .wav  .mp3  .flac  .ogg  .aac  .m4a   (EOF 追加)
文档：  .pdf                                   (%%EOF 追加)
```

---

## 快速开始

```bash
# 编译
go build -o steggo.exe ./cmd/steggo

# 把 secret.txt 藏进 photo.png
steggo embed -c photo.png -s secret.txt -p "YourPassword" -o steg.png

# 从 steg.png 里提取出来
steggo extract -c steg.png -p "YourPassword" -o ./out/

# 验证 steg.png 是否被篡改
steggo verify -c steg.png -H steg.png.sha256

# 批量嵌入整个目录的 PNG/MP3/PDF
steggo batch-embed -d ./photos/ -s secret.txt -p "YourPassword" -o ./steg_photos/

# 批量提取
steggo batch-extract -d ./steg_photos/ -p "YourPassword" -o ./secrets/
```

---

## 安全设计

```
载体文件
    └── 嵌入层（PNG LSB / EOF追加）
            └── 载荷结构
                    [magic 8B] [name_len 2B] [filename] [data_len 4B] [AES密文]
                                                                          │
                                                          AES-256-GCM 加密
                                                          ┌─────────────────┐
                                                          │ PBKDF2 密钥派生  │
                                                          │ salt(16B 随机)   │
                                                          │ nonce(12B 随机)  │
                                                          │ GCM 认证标签     │
                                                          └─────────────────┘
```

- 每次 embed 使用**随机 salt + 随机 nonce**，相同内容+密码输出结果不同
- 错误密码会触发 GCM **认证失败**，而非返回乱码明文
- 输出 `.sha256` 文件用于后续**完整性校验**

---

## 项目结构

```
StegGo/
├── cmd/steggo/main.go              # Cobra CLI 入口
├── internal/
│   ├── crypto/aes.go               # AES-256-GCM + PBKDF2
│   ├── hash/hash.go                # SHA-256 文件哈希
│   └── steganography/
│       ├── payload.go              # 载荷二进制编解码
│       ├── image.go                # PNG LSB + JPEG EOF追加
│       ├── audio.go                # WAV/MP3等 EOF追加
│       ├── pdf.go                  # PDF EOF追加
│       └── steg.go                 # 高层 Embed/Extract/Batch
├── go.mod
└── README.md
```

---

## 依赖

- `golang.org/x/crypto` — PBKDF2 密钥派生
- `github.com/spf13/cobra` — CLI 框架
- Go 标准库：`image/png`、`crypto/aes`、`crypto/cipher`、`encoding/binary` 等
