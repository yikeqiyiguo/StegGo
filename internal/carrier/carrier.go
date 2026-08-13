// Package carrier 提供 StegGo V2.0 载体层：
//
//	格式识别（魔数 + 扩展名）、有损黑名单拦截、载体容量探测、
//	字节级载荷嵌入/提取（图像 / 尾部容器 / 零宽文本）、
//	Polyglot 多格式拼接、套娃递归嵌套。
//
// 本层依赖 internal/common 与 internal/algorithm：
//   - 载荷为字节流（加密与封装由 internal/crypto 完成，本层不关心载荷语义）
//   - 图像载体委托 internal/algorithm 完成位级嵌入
//   - 尾部容器（WAV/PDF/视频）将载荷追加到文件末尾并带 V2.0 魔数标记
//   - 有损压缩格式（JPG/MP3 等）一律拦截，与三端黑名单一致
package carrier

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"steggo/internal/algorithm"
	"steggo/internal/common"
)

// Kind 载体类型。
type Kind int

const (
	KindUnknown Kind = iota
	KindImage        // PNG / BMP / TIFF（无损位图）
	KindAudio        // WAV / FLAC（尾部容器）
	KindPDF          // PDF（尾部容器）
	KindText         // TXT / MD（零宽字符）
	KindVideo        // MP4 / MKV / WEBM / AVI（尾部容器）
)

func (k Kind) String() string {
	switch k {
	case KindImage:
		return "image"
	case KindAudio:
		return "audio"
	case KindPDF:
		return "pdf"
	case KindText:
		return "text"
	case KindVideo:
		return "video"
	default:
		return "unknown"
	}
}

// 载体层错误。
var (
	// ErrLossyFormat 有损压缩格式禁止作为载体（会破坏隐写数据）。
	ErrLossyFormat = errors.New("有损压缩格式禁止作为载体 (JPG/MP3/AAC/OGG/M4A/WMA)")
	// ErrUnsupportedFormat 不支持的载体格式。
	ErrUnsupportedFormat = errors.New("不支持的载体格式")
	// ErrNoPayload 载体中未找到 StegGo 载荷。
	ErrNoPayload = errors.New("载体中未找到 StegGo 载荷 (可能密码错误或未嵌入)")
	// ErrCorrupted 载荷头部损坏。
	ErrCorrupted = errors.New("载荷头部损坏")
	// ErrTooLarge 载荷超过容器上限。
	ErrTooLarge = errors.New("载荷超过容器容量上限")
)

// maxTailPayload 尾部容器允许的最大载荷字节数（防止恶意文件导致 OOM）。
const maxTailPayload = 512 << 20 // 512 MiB

// Options 载体层统一参数。
// 算法参数与 algorithm.Options 平铺，便于 CLI/TUI/GUI 层直接构造。
type Options struct {
	// Algorithm 图像算法名称：lsb|dct|dwt|hugo|wow|uniward（默认 lsb）。
	Algorithm string
	// TailTag 尾部容器标记；为空时使用 common.MagicV3。
	TailTag []byte

	// ---- 算法参数（与 internal/algorithm.Options 平铺）----
	BitDepth    int     // LSB：每通道嵌入位数 1-4（默认 1）
	ChannelMask int     // LSB：通道掩码 bit0=R bit1=G bit2=B（默认全开）
	BlockSize   int     // DCT/DWT：块大小（默认 8）
	Quality     int     // DCT：量化步长 1-32（默认 8）
	Levels      int     // DWT：分解级数 1-3（默认 2）
	CostStyle   string  // 自适应：成本函数 hill|wow|uniward
	Gamma       float64 // 自适应：成本指数（默认 1）
	Seed        []byte  // 伪随机游走种子（由上层密码派生）
}

// AlgorithmOptions 转换为算法层参数。
func (o Options) AlgorithmOptions() algorithm.Options {
	return algorithm.Options{
		BitDepth:    o.BitDepth,
		ChannelMask: o.ChannelMask,
		BlockSize:   o.BlockSize,
		Quality:     o.Quality,
		Levels:      o.Levels,
		CostStyle:   o.CostStyle,
		Gamma:       o.Gamma,
		Seed:        o.Seed,
	}
}

// fillDefaults 填充默认参数并校验（含写回归一化结果）。
func (o *Options) fillDefaults() error {
	if o.Algorithm == "" {
		o.Algorithm = "lsb"
	}
	if len(o.TailTag) == 0 {
		o.TailTag = []byte(common.MagicV3)
	}
	if algorithm.Get(o.Algorithm) == nil {
		return fmt.Errorf("未知算法: %s", o.Algorithm)
	}
	ao := o.AlgorithmOptions()
	if err := ao.Normalize(); err != nil {
		return err
	}
	o.BitDepth, o.ChannelMask = ao.BitDepth, ao.ChannelMask
	o.BlockSize, o.Quality, o.Levels = ao.BlockSize, ao.Quality, ao.Levels
	o.CostStyle, o.Gamma = ao.CostStyle, ao.Gamma
	return nil
}

// Carrier 载体统一接口（面向字节载荷）。
type Carrier interface {
	// Kind 载体类型。
	Kind() Kind
	// Extensions 支持的扩展名列表（小写）。
	Extensions() []string
	// Capacity 计算载体可容纳的载荷字节数。
	Capacity(path string, opt Options) (int64, error)
	// HasCapacity 判断载体是否足以容纳 size 字节。
	HasCapacity(path string, size int64, opt Options) (bool, error)
	// Embed 将载荷嵌入载体文件，输出到 outPath。
	Embed(path, outPath string, payload []byte, opt Options) error
	// Extract 从载体提取载荷字节流。
	Extract(path string, opt Options) ([]byte, error)
}

// registry 载体注册表（按 Kind 注册，插件化）。
var registry = map[Kind]Carrier{}

// Register 注册载体实现。
func Register(c Carrier) {
	if c == nil {
		return
	}
	registry[c.Kind()] = c
}

// Get 按类型获取载体实现；未注册返回 nil。
func Get(kind Kind) Carrier { return registry[kind] }

// Kinds 返回已注册的载体类型（稳定顺序）。
func Kinds() []Kind {
	return []Kind{KindImage, KindAudio, KindPDF, KindText, KindVideo}
}

// ForPath 依据文件格式返回对应的载体实现。
func ForPath(path string) (Carrier, error) {
	kind, err := DetectKind(path)
	if err != nil {
		return nil, err
	}
	c := Get(kind)
	if c == nil {
		return nil, fmt.Errorf("%w: 未注册的载体类型 %v", ErrUnsupportedFormat, kind)
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// 格式识别
// ---------------------------------------------------------------------------

// 文件魔数。
var (
	magicPNG  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	magicJPEG = []byte{0xFF, 0xD8, 0xFF}
	magicGIF  = []byte("GIF8")
	magicTIFF = [][]byte{{'I', 'I', 0x2A, 0x00}, {'M', 'M', 0x00, 0x2A}}
	magicRIFF = []byte("RIFF")
	magicWAVE = []byte("WAVE")
	magicAVI  = []byte("AVI ")
	magicPDF  = []byte("%PDF-")
	magicFLAC = []byte("fLaC")
	magicID3  = []byte("ID3")
	magicZIP  = []byte{'P', 'K', 0x03, 0x04}
	magicEBML = []byte{0x1A, 0x45, 0xDF, 0xA3} // MKV / WEBM
	magicFtyp = []byte("ftyp")                // MP4（偏移 4）
)

// extKind 扩展名兜底映射（魔数无法识别时的后备判断）。
var extKind = map[string]Kind{
	".png": KindImage, ".bmp": KindImage, ".tif": KindImage, ".tiff": KindImage,
	".wav": KindAudio, ".flac": KindAudio,
	".pdf": KindPDF,
	".txt": KindText, ".md": KindText, ".markdown": KindText,
	".mp4": KindVideo, ".mkv": KindVideo, ".webm": KindVideo, ".avi": KindVideo,
}

// isLossyExt 判断扩展名是否属于有损黑名单。
func isLossyExt(ext string) bool {
	for _, e := range common.LossyBlacklist {
		if ext == e {
			return true
		}
	}
	return false
}

// readHeader 读取文件头部 n 字节。
func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := make([]byte, n)
	m, err := f.Read(h)
	if err != nil && m == 0 {
		return nil, err
	}
	return h[:m], nil
}

// sniffHeader 仅依据文件头魔数识别真实格式。
// 返回 (kind, err)；err 为 nil 表示该格式可用作载体。
func sniffHeader(h []byte) (Kind, error) {
	if len(h) >= 8 && bytes.Equal(h[:8], magicPNG) {
		return KindImage, nil
	}
	if len(h) >= 2 && h[0] == 'B' && h[1] == 'M' {
		return KindImage, nil
	}
	for _, m := range magicTIFF {
		if len(h) >= 4 && bytes.Equal(h[:4], m) {
			return KindImage, nil
		}
	}
	if len(h) >= 12 && bytes.Equal(h[:4], magicRIFF) && bytes.Equal(h[8:12], magicWAVE) {
		return KindAudio, nil
	}
	if len(h) >= 4 && bytes.Equal(h[:4], magicFLAC) {
		return KindAudio, nil
	}
	if len(h) >= 5 && bytes.Equal(h[:5], magicPDF) {
		return KindPDF, nil
	}
	if len(h) >= 12 && bytes.Equal(h[4:8], magicFtyp) {
		return KindVideo, nil
	}
	if len(h) >= 4 && bytes.Equal(h[:4], magicEBML) {
		return KindVideo, nil
	}
	if len(h) >= 12 && bytes.Equal(h[:4], magicRIFF) && bytes.Equal(h[8:12], magicAVI) {
		return KindVideo, nil
	}
	// 有损格式：即使扩展名伪装成其他格式也必须拦截。
	if len(h) >= 3 && bytes.Equal(h[:3], magicJPEG) {
		return KindUnknown, ErrLossyFormat
	}
	if len(h) >= 3 && bytes.Equal(h[:3], magicID3) {
		return KindUnknown, ErrLossyFormat
	}
	// 已知但不支持的格式。
	if len(h) >= 4 && bytes.Equal(h[:4], magicGIF) {
		return KindUnknown, ErrUnsupportedFormat
	}
	return KindUnknown, ErrUnsupportedFormat
}

// DetectKind 检测载体类型。
//
// 识别顺序：文件头魔数（权威）→ 扩展名兜底。
// 有损格式（JPG/MP3/AAC/OGG/M4A/WMA）无论魔数还是扩展名命中一律拦截；
// 魔数已明确识别为有损格式时，不再回退扩展名（防止伪装绕过）。
func DetectKind(path string) (Kind, error) {
	h, err := readHeader(path, 16)
	if err != nil {
		return KindUnknown, err
	}
	kind, serr := sniffHeader(h)
	if serr == nil {
		return kind, nil
	}
	if errors.Is(serr, ErrLossyFormat) {
		return KindUnknown, serr
	}
	// 魔数无法识别（如纯文本）：按扩展名兜底。
	ext := strings.ToLower(filepath.Ext(path))
	if kind, ok := extKind[ext]; ok {
		return kind, nil
	}
	if isLossyExt(ext) {
		return KindUnknown, ErrLossyFormat
	}
	return KindUnknown, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Base(path))
}

// DetectKindBytes 依据字节流前 16 字节检测载体类型（供套娃中间层使用）。
func DetectKindBytes(data []byte) (Kind, error) {
	if len(data) > 16 {
		data = data[:16]
	}
	return sniffHeader(data)
}

// IsSupportedExt 判断扩展名是否属于已知载体格式（不检查文件真实性）。
func IsSupportedExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if isLossyExt(ext) {
		return false
	}
	_, ok := extKind[ext]
	return ok
}
