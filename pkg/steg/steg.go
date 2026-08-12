// Package steg 提供 StegGo 隐写核心 SDK。
//
//	嵌入链路：明文 → ZIP压缩 → PBKDF2派生 → AES-256-GCM加密 → SHA256绑定 → 抗检测LSB/载体容器
//	提取链路：抗检测LSB/载体容器 → SHA256校验 → AES-GCM解密 → 解ZIP → 还原明文
package steg

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"steggo/pkg/carrier"
	"steggo/pkg/crypto"
)

// decryptStreamBlocks 解密流式分块容器：[len u32][enc chunk]...
func decryptStreamBlocks(cipher []byte, key []byte) ([]byte, error) {
	var out []byte
	off := 0
	for off < len(cipher) {
		if off+4 > len(cipher) {
			return nil, errors.New("流式密文块头部损坏")
		}
		n := int(binary.BigEndian.Uint32(cipher[off : off+4]))
		off += 4
		if n <= 0 || off+n > len(cipher) {
			return nil, errors.New("流式密文块长度非法")
		}
		pt, err := crypto.DecryptChunk(cipher[off:off+n], key)
		if err != nil {
			return nil, err
		}
		out = append(out, pt...)
		off += n
	}
	if len(out) == 0 {
		return nil, errors.New("流式密文为空")
	}
	return out, nil
}

// DefaultBitDepth 默认每通道嵌入位数。
const DefaultBitDepth = 2

// zipThreshold 超过该字节数的明文启用 ZIP 压缩。
const zipThreshold = 512

// Options 隐写选项。
type Options struct {
	// BitDepth 图片 LSB 每通道嵌入位数，范围 1-4，默认 2。
	BitDepth int
	// Password 加密密码，必填。
	Password []byte
	// Name 覆盖保存的文件名（默认取 secretPath 文件名）。
	Name string
	// DirMode 将 secretPath 作为目录整体打包嵌入。
	DirMode bool
	// SkipZIP 禁用 ZIP 压缩。
	SkipZIP bool
}

// Result 一次隐写操作的结果信息。
type Result struct {
	Name    string
	IsZIP   bool
	IsDir   bool
	RawSize int64
	Meta    *Meta
}

// normalize 校验并填充默认值。
func (o *Options) normalize() error {
	if len(o.Password) == 0 {
		return errors.New("密码不能为空")
	}
	if o.BitDepth == 0 {
		o.BitDepth = DefaultBitDepth
	}
	if o.BitDepth < 1 || o.BitDepth > 4 {
		return errors.New("嵌入位数必须在 1-4 之间")
	}
	return nil
}

// BuildSecretPayload 构造加密载荷：读数据 → 压缩 → 加密 → 哈希绑定 → 组装。
// 返回完整载荷（头部+密文）与元信息。
func BuildSecretPayload(secretPath string, password []byte, opts Options) ([]byte, *Header, error) {
	if err := opts.normalize(); err != nil {
		return nil, nil, err
	}

	var (
		raw      []byte
		name     string
		isDir    bool
		rawSize  int64
	)

	if opts.DirMode {
		info, err := os.Stat(secretPath)
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			return nil, nil, errors.New("DirMode 要求 secretPath 为目录")
		}
		z, err := crypto.ZipDir(secretPath)
		if err != nil {
			return nil, nil, err
		}
		raw = z
		name = opts.Name
		if name == "" {
			name = filepath.Base(secretPath)
		}
		isDir = true
		rawSize = 0
		if err := filepath.Walk(secretPath, func(_ string, fi os.FileInfo, _ error) error {
			if !fi.IsDir() {
				rawSize += fi.Size()
			}
			return nil
		}); err != nil {
			rawSize = int64(len(z))
		}
	} else {
		data, err := os.ReadFile(secretPath)
		if err != nil {
			return nil, nil, err
		}
		raw = data
		name = opts.Name
		if name == "" {
			name = filepath.Base(secretPath)
		}
		rawSize = int64(len(data))
	}
	defer crypto.Wipe(raw)

	// ZIP 压缩
	flags := byte(0)
	plain := raw
	if !opts.SkipZIP {
		if z, ok, err := crypto.MaybeZip(raw, zipThreshold); err == nil && ok {
			plain = z
			flags |= flagZIP
		}
	}
	if isDir {
		flags |= flagDir
	}
	defer crypto.Wipe(plain)

	// AES-GCM 加密
	enc, err := crypto.Encrypt(plain, password)
	if err != nil {
		return nil, nil, err
	}
	defer crypto.Wipe(enc)

	h := &Header{
		Version:   versionV2,
		Flags:     flags,
		BitDepth:  opts.BitDepth,
		Name:      name,
		Salt:      append([]byte(nil), enc[:crypto.SaltSize]...),
		Nonce:     append([]byte(nil), enc[crypto.SaltSize:crypto.SaltSize+crypto.NonceSize]...),
		CipherLen: len(enc) - crypto.SaltSize - crypto.NonceSize,
	}
	cipher := enc[crypto.SaltSize+crypto.NonceSize:]
	sum := sha256.Sum256(cipher)
	h.CipherSum = sum

	payload := BuildPayload(h, cipher)
	return payload, h, nil
}

// ParseSecretPayload 解析并校验载荷：SHA256 绑定校验 → 解密 → 解压。
func ParseSecretPayload(payload []byte, password []byte) (plain []byte, h *Header, err error) {
	if len(payload) < 8 || string(payload[:8]) != string(MagicV2) {
		return nil, nil, errors.New("非 StegGo V2 载荷")
	}
	h, headerLen, err := ParseHeader(payload)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateHeader(h); err != nil {
		WipeHeader(h)
		return nil, nil, err
	}
	if len(payload) < headerLen+h.CipherLen {
		WipeHeader(h)
		return nil, nil, errors.New("载荷数据不完整")
	}
	cipher := payload[headerLen : headerLen+h.CipherLen]

	// SHA256 完整性校验（防篡改）
	sum := sha256.Sum256(cipher)
	if !crypto.ConstantTimeEqual(sum[:], h.CipherSum[:]) {
		WipeHeader(h)
		return nil, nil, errors.New("载荷完整性校验失败：载体可能已被篡改")
	}

	// 派生密钥并解密
	key := crypto.DeriveKey(password, h.Salt, crypto.DefaultIterations)
	defer crypto.Wipe(key)
	var pt []byte
	if h.Flags&flagStream != 0 {
		// 流式分块容器：逐块解密
		pt, err = decryptStreamBlocks(cipher, key)
	} else {
		pt, err = crypto.DecryptWithKey(cipher, key, h.Nonce)
	}
	if err != nil {
		WipeHeader(h)
		return nil, nil, err
	}
	if h.Flags&flagStream != 0 {
		// 流式模式已直接得到明文，无 ZIP
		return pt, h, nil
	}
	// 解压（若标记 ZIP）
	if h.Flags&flagZIP != 0 {
		if !crypto.IsZip(pt) {
			WipeHeader(h)
			crypto.Wipe(pt)
			return nil, nil, errors.New("载荷标记 ZIP 但数据不是有效压缩包")
		}
		plain, err = crypto.UnzipBytesToMemory(pt)
		crypto.Wipe(pt)
		if err != nil {
			WipeHeader(h)
			return nil, nil, err
		}
		return plain, h, nil
	}
	return pt, h, nil
}

// BuildGenericPayload 构造通用（非图片）载体载荷并解密辅助。
func BuildGenericPayload(secretPath string, password []byte, opts Options) ([]byte, *Header, error) {
	return BuildSecretPayload(secretPath, password, opts)
}

// ParseGenericPayload 解析通用载体载荷。
func ParseGenericPayload(payload []byte, password []byte) ([]byte, *Header, error) {
	return ParseSecretPayload(payload, password)
}

// =============================================================
// 图片载体（抗检测 LSB）
// =============================================================

// EmbedImage 将文件/目录嵌入无损图片载体。
func EmbedImage(carrierPath, outputPath, secretPath string, opts Options) (*Result, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	payload, _, err := BuildSecretPayload(secretPath, opts.Password, opts)
	if err != nil {
		return nil, err
	}
	defer crypto.Wipe(payload)

	img, err := carrier.LoadImage(carrierPath)
	if err != nil {
		return nil, err
	}
	capacity := CapLSBBytes(img, opts.BitDepth)
	need := (len(payload) + 8 + 7) / 8 * 8
	if need > capacity {
		return nil, fmt.Errorf("载体容量不足：需要约 %d B，载体最大 %d B（位深度 %d）", len(payload), capacity, opts.BitDepth)
	}

	seed := SeedFromPassword(opts.Password)
	defer crypto.Wipe(seed)
	if err := EmbedLSB(img, ByteToBits(payload), seed, opts.BitDepth); err != nil {
		return nil, err
	}
	if err := carrier.SaveImage(img, outputPath); err != nil {
		return nil, err
	}
	return &Result{
		Name:    filepath.Base(secretPath),
		RawSize: int64(len(payload)),
	}, nil
}

// ExtractImage 从图片载体提取并还原数据。
func ExtractImage(carrierPath, outputDir string, password []byte) (*Result, error) {
	if len(password) == 0 {
		return nil, errors.New("密码不能为空")
	}
	img, err := carrier.LoadImage(carrierPath)
	if err != nil {
		return nil, err
	}
	seed := SeedFromPassword(password)
	defer crypto.Wipe(seed)

	// 依次尝试 4/3/2/1 位深度
	for depth := 4; depth >= 1; depth-- {
		stream, err := ExtractLSB(img, seed, depth)
		if err != nil {
			continue
		}
		if len(stream) < 64 {
			continue
		}
		payload := BitsToBytes(stream)
		if len(payload) < 8 || string(payload[:8]) != string(MagicV2) {
			continue
		}
		plain, h, err := ParseSecretPayload(payload, password)
		if err != nil {
			return nil, err
		}
		if err := writeExtracted(plain, h, outputDir); err != nil {
			WipeHeader(h)
			crypto.Wipe(plain)
			return nil, err
		}
		res := &Result{Name: h.Name, IsZIP: h.Flags&flagZIP != 0, IsDir: h.Flags&flagDir != 0, RawSize: int64(len(plain)), Meta: &Meta{Name: h.Name, IsZIP: h.Flags&flagZIP != 0, IsDir: h.Flags&flagDir != 0, BitDepth: depth}}
		WipeHeader(h)
		crypto.Wipe(plain)
		return res, nil
	}

	// V2 失败 → 尝试 V1 旧版兼容
	if legacy, ok := tryLegacyImage(img, password); ok {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return nil, err
		}
		name, data, err := decodeLegacy(legacy)
		if err != nil {
			return nil, err
		}
		plain, err := legacyDecrypt(data, password)
		crypto.Wipe(data)
		if err != nil {
			return nil, err
		}
		outPath := filepath.Join(outputDir, name)
		if err := os.WriteFile(outPath, plain, 0600); err != nil {
			crypto.Wipe(plain)
			return nil, err
		}
		res := &Result{Name: name, RawSize: int64(len(plain))}
		crypto.Wipe(plain)
		return res, nil
	}

	return nil, errors.New("未检测到有效的 StegGo 载荷（请确认密码正确且载体未被破坏）")
}

// =============================================================
// 通用载体（音频/PDF/视频/文本零宽）
// =============================================================

// EmbedGeneric 将文件嵌入通用载体（音频/PDF/视频）。
func EmbedGeneric(carrierPath, outputPath, secretPath string, opts Options) (*Result, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	payload, _, err := BuildSecretPayload(secretPath, opts.Password, opts)
	if err != nil {
		return nil, err
	}
	defer crypto.Wipe(payload)

	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	switch kind {
	case carrier.KindAudio:
		err = carrier.EmbedWAV(carrierPath, outputPath, payload)
	case carrier.KindPDF:
		err = carrier.EmbedPDF(carrierPath, outputPath, payload)
	case carrier.KindVideo:
		err = carrier.EmbedVideo(carrierPath, outputPath, payload, 0)
	default:
		return nil, errors.New("该载体类型不支持通用隐写")
	}
	if err != nil {
		return nil, err
	}
	return &Result{Name: filepath.Base(secretPath)}, nil
}

// ExtractGeneric 从通用载体提取并还原数据。
func ExtractGeneric(carrierPath, outputDir string, password []byte) (*Result, error) {
	if len(password) == 0 {
		return nil, errors.New("密码不能为空")
	}
	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	var payload []byte
	switch kind {
	case carrier.KindAudio:
		payload, err = carrier.ExtractWAV(carrierPath)
	case carrier.KindPDF:
		payload, err = carrier.ExtractPDF(carrierPath)
	case carrier.KindVideo:
		payload, err = carrier.ExtractVideo(carrierPath)
	default:
		return nil, errors.New("该载体类型不支持通用提取")
	}
	if err != nil {
		return nil, err
	}
	plain, h, err := ParseSecretPayload(payload, password)
	crypto.Wipe(payload)
	if err != nil {
		return nil, err
	}
	if err := writeExtracted(plain, h, outputDir); err != nil {
		WipeHeader(h)
		crypto.Wipe(plain)
		return nil, err
	}
	res := &Result{Name: h.Name, IsZIP: h.Flags&flagZIP != 0, IsDir: h.Flags&flagDir != 0, RawSize: int64(len(plain))}
	WipeHeader(h)
	crypto.Wipe(plain)
	return res, nil
}

// EmbedText 将文件嵌入文本载体（零宽字符隐写）。
func EmbedText(carrierPath, outputPath, secretPath string, opts Options) (*Result, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	payload, _, err := BuildSecretPayload(secretPath, opts.Password, opts)
	if err != nil {
		return nil, err
	}
	defer crypto.Wipe(payload)

	text, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	out := carrier.EncodeZeroWidth(string(text), payload)
	return &Result{Name: filepath.Base(secretPath)}, os.WriteFile(outputPath, []byte(out), 0644)
}

// ExtractText 从文本载体提取数据。
func ExtractText(carrierPath, outputDir string, password []byte) (*Result, error) {
	text, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	payload, err := carrier.DecodeZeroWidth(string(text))
	if err != nil {
		return nil, err
	}
	defer crypto.Wipe(payload)
	plain, h, err := ParseSecretPayload(payload, password)
	if err != nil {
		return nil, err
	}
	if err := writeExtracted(plain, h, outputDir); err != nil {
		WipeHeader(h)
		crypto.Wipe(plain)
		return nil, err
	}
	res := &Result{Name: h.Name, IsZIP: h.Flags&flagZIP != 0, IsDir: h.Flags&flagDir != 0, RawSize: int64(len(plain))}
	WipeHeader(h)
	crypto.Wipe(plain)
	return res, nil
}

// writeExtracted 将解出的明文写入输出目录（目录打包时整体解压）。
func writeExtracted(plain []byte, h *Header, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	if h.Flags&flagDir != 0 {
		return crypto.UnzipBytes(plain, outputDir)
	}
	if h.Flags&flagZIP != 0 {
		// 单文件 zip：解压到输出目录，保留原始文件名
		name, data, err := crypto.UnzipSingleFile(plain)
		if err != nil {
			return err
		}
		crypto.Wipe(plain)
		if name == "" {
			name = h.Name
		}
		return os.WriteFile(filepath.Join(outputDir, name), data, 0600)
	}
	return os.WriteFile(filepath.Join(outputDir, h.Name), plain, 0600)
}

// autoDetectAndEmbed 根据载体类型自动分发嵌入（供 CLI 使用）。
func AutoEmbed(carrierPath, outputPath, secretPath string, opts Options) (*Result, error) {
	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	switch kind {
	case carrier.KindImage:
		return EmbedImage(carrierPath, outputPath, secretPath, opts)
	case carrier.KindAudio, carrier.KindPDF, carrier.KindVideo:
		return EmbedGeneric(carrierPath, outputPath, secretPath, opts)
	case carrier.KindText:
		return EmbedText(carrierPath, outputPath, secretPath, opts)
	}
	return nil, errors.New("不支持的载体类型")
}

// AutoExtract 根据载体类型自动分发提取（供 CLI 使用）。
func AutoExtract(carrierPath, outputDir string, password []byte) (*Result, error) {
	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	switch kind {
	case carrier.KindImage:
		return ExtractImage(carrierPath, outputDir, password)
	case carrier.KindAudio, carrier.KindPDF, carrier.KindVideo:
		return ExtractGeneric(carrierPath, outputDir, password)
	case carrier.KindText:
		return ExtractText(carrierPath, outputDir, password)
	}
	return nil, errors.New("不支持的载体类型")
}

var _ image.Image
