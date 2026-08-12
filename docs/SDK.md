# StegGo Go SDK 文档

StegGo 以模块 `steggo` 提供可直接引用的 Go SDK，覆盖加密、隐写、载体、任务调度全链路。

```go
import (
    "steggo/pkg/crypto"
    "steggo/pkg/steg"
    "steggo/pkg/carrier"
    "steggo/pkg/task"
)
```

---

## 1. `pkg/crypto` — 加密与压缩

### 核心加解密

```go
// 一次性加密/解密（自动内嵌 salt + nonce，密文自包含）
ct, err := crypto.Encrypt(plain []byte, password []byte)   // ct = salt|nonce|ciphertext|tag
pt, err := crypto.Decrypt(ct []byte, password []byte)

// 分片加密（流式场景，密钥由 DeriveKey 派生后复用）
key  := crypto.DeriveKey(password, salt, crypto.DefaultIterations) // 210,000 次
ct2, err := crypto.EncryptChunk(plain []byte, key []byte)
pt2, err := crypto.DecryptChunk(ct2 []byte, key []byte)
```

关键常量：`SaltSize=16`、`NonceSize=12`、`KeySize=32`、`TagSize=16`、`DefaultIterations=210000`。

> 银行级默认值：PBKDF2-SHA256 × 210,000 次迭代 + AES-256-GCM 认证加密。

### 内存清零

```go
crypto.Wipe(buf []byte) // 将缓冲区清零，用于密码/密钥/明文的及时销毁
```

### ZIP 压缩

```go
z, err := crypto.ZipBytes(data []byte, name string)   // 单文件压缩
name, data, err := crypto.UnzipSingleFile(z []byte)   // 单文件解压
z, err := crypto.ZipDir(dir string)                   // 目录压缩
err := crypto.UnzipBytes(z []byte, destDir string)    // 解压到目录（含 Zip-Slip 防护）
out, zipped, err := crypto.MaybeZip(data []byte, minLen int) // 超过阈值才压缩
b := crypto.IsZip(data []byte)                        // 识别 zip 头
```

---

## 2. `pkg/steg` — 隐写核心

### 高级入口（推荐）

```go
// 自动识别载体类型并嵌入/提取
res, err := steg.AutoEmbed(carrierPath, outputPath, secretPath string, steg.Options{
    Password: []byte("..."),
    BitDepth: 2, // 图片 LSB 位深度 1-4
})
res, err := steg.AutoExtract(carrierPath, outputDir string, password []byte)

// Result 字段
//   res.Name    提取出的文件名
//   res.RawSize 明文原始大小
//   res.BitDepth 使用的位深度
```

### 抗检测 LSB（图片）

```go
img, _ := carrier.LoadImage("photo.png")        // 仅接受无损格式
bits := steg.ByteToBits(secretData)             // 字节 → 位流
err := steg.EmbedLSB(img, bits, seed, 2)        // 伪随机坐标嵌入
stream, err := steg.ExtractLSB(img, seed, 2)    // 提取位流
data := steg.BitsToBytes(stream)
seed := steg.SeedFromPassword(password)         // 坐标种子派生
capBytes := steg.CapLSBBytes(img, 2)            // 容量预检
```

### 自检审计

```go
res, err := steg.AuditImage("photo.png")
//   res.ChiSquare.PValue      卡方检验 p 值
//   res.RS.EstimatedRate      RS 嵌入率估计 (0-1)
//   res.SPA.Skew              SPA 偏斜度
//   res.Details []string      明细说明
//   res.Verdict               综合判定（干净/低纹理保护/可疑...）
```

### 质量与容量

```go
psnr, err := steg.PSNR(origImg, stegImg)          // 相同图返回 +Inf
ssim := steg.SSIM(origImg, stegImg)               // 相同图返回 1
r, err := steg.CheckImageCapacity(path, 2)        // 单档容量
m, err := steg.CapacityMatrix(path)               // 1-4 位深度矩阵
```

### Shamir 门限分片

```go
shares, err := steg.SplitSecret(data, 5, 3)       // 5 片、门限 3
plain, err := steg.RecoverSecret(shares[:3], 3)   // 任意 3 片恢复
```

分片首字节为 x 坐标，因此分片可任意挑选、任意顺序，实现真正的"凑齐 k 片"。

### 批量与流式

```go
res, err := steg.BatchEmbed(ctx, carrierDir, outDir, secretPath, steg.BatchEmbedOptions{
    Options:     steg.Options{Password: pw, BitDepth: 2},
    Concurrency: 4,
})
res, err := steg.BatchExtract(ctx, carrierDir, outDir, pw, steg.BatchExtractOptions{Concurrency: 4})
// 每个 res[i].Error 记录单文件成败

err := steg.StreamEmbedGeneric(carrierPath, outputPath, payload []byte, opts) // 64MB 分块
payload, err := steg.StreamExtractGeneric(carrierPath, opts)
```

### 旧版兼容

`pkg/steg/legacy.go` 提供对 V1 老格式（`STEGGO01`，100k 迭代）的自动识别与提取，升级无缝。

---

## 3. `pkg/carrier` — 载体识别

```go
kind, err := carrier.DetectKind("photo.png")   // KindImage / KindAudio / KindPDF / KindText / KindVideo
ok := carrier.IsSupported("photo.jpg")         // false：有损格式
img, err := carrier.LoadImage("photo.png")     // PNG/BMP/TIFF；JPG 文件头直接拦截
err := carrier.SaveImage(img, "out.png")
info, err := carrier.GetImageInfo("photo.png") // Width/Height/Channels
```

---

## 4. `pkg/task` — 任务队列

```go
q := task.New(4) // 并发 worker 池
results := q.Run(context.Background(), []task.Task{
    {ID: 1, Name: "a", Fn: func() error { return nil }},
})
// results[i].Error 记录任务错误；ctx 取消时优雅停止
```

---

## 完整示例：SDK 嵌入 → 审计 → 提取

```go
package main

import (
    "fmt"
    "steggo/pkg/crypto"
    "steggo/pkg/steg"
)

func main() {
    pw := []byte("bank-grade-password")

    // 1. 嵌入
    res, err := steg.AutoEmbed("photo.png", "steg.png", "secret.zip", steg.Options{Password: pw, BitDepth: 2})
    if err != nil { panic(err) }
    fmt.Println("embedded:", res.Name, res.RawSize)

    // 2. 自检审计
    audit, err := steg.AuditImage("steg.png")
    if err != nil { panic(err) }
    fmt.Println("audit:", audit.Verdict)

    // 3. 提取
    out, err := steg.AutoExtract("steg.png", "./recovered", pw)
    if err != nil { panic(err) }
    fmt.Println("recovered:", out.Name, out.RawSize)
    crypto.Wipe(pw)
}
```

---

## 工程约定

- **仅无损载体**：有损格式（JPG/MP3/FLAC）在 `carrier.DetectKind` 层即被拦截。
- **内存安全**：密码/密钥/明文使用后调用 `crypto.Wipe` 清零。
- **默认安全**：PBKDF2 迭代数、GCM 随机数、盐均由库默认保证，调用方无需配置。
- **向后兼容**：SDK 自动识别 V1 老格式并提取。
