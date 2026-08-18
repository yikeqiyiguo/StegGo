// Package algorithm 提供 StegGo V2.0 隐写算法层：
//
//	LSB（抗检测伪随机游走 + 多深度）
//	DCT（8×8 频域中频系数 QIM）
//	DWT（整数 Haar 小波提升 + 高频奇偶嵌入）
//	HUGO / WOW / UNIWARD（失真成本自适应，复杂区域优先）
//	载体分析（容量 / PSNR / SSIM / 卡方检验 / RS 分析）
//
// 本层不依赖任何上层包，仅依赖标准库，保证零循环依赖。
// 所有算法输入为 0/1 位流（每字节 0 或 1），加密与封装由 internal/crypto 完成。
package algorithm

import (
	"errors"
	"fmt"
	"image"
	"sort"
)

// AlgoID 与 internal/crypto 的算法标识保持一致。
const (
	IDLSB      byte = 0
	IDDCT      byte = 1
	IDDWT      byte = 2
	IDHUGO     byte = 3
	IDWOW      byte = 4
	IDUNIWARD  byte = 5
	IDANCHORED byte = 6
)

// Options 算法统一参数。
type Options struct {
	BitDepth    int     // LSB/自适应：每通道嵌入位数 1-4（自适应固定 1）
	ChannelMask int     // LSB 通道掩码：bit0=R bit1=G bit2=B，0 表示全开
	BlockSize   int     // DCT/DWT 块大小（默认 8）
	Quality     int     // DCT 量化步长 1-32（默认 8，越大越鲁棒但失真越大）
	Levels      int     // DWT 分解级数 1-3（默认 2）
	CostStyle   string  // 自适应成本函数："hill"|"wow"|"uniward"
	Gamma       float64 // 自适应成本指数（默认 1）
	Seed        []byte  // 伪随机游走种子（由上层密码派生）
}

// fillDefaults 填充默认参数并校验。
func (o *Options) fillDefaults() error {
	if o.BitDepth == 0 {
		o.BitDepth = 1
	}
	if o.BitDepth < 1 || o.BitDepth > 4 {
		return errors.New("嵌入位数必须在 1-4 之间")
	}
	if o.ChannelMask == 0 {
		o.ChannelMask = 0b111
	}
	if o.ChannelMask < 1 || o.ChannelMask > 0b111 {
		return errors.New("通道掩码非法")
	}
	if o.BlockSize == 0 {
		o.BlockSize = 8
	}
	if o.Quality == 0 {
		o.Quality = 8
	}
	if o.Quality < 1 {
		o.Quality = 1
	}
	if o.Levels == 0 {
		o.Levels = 2
	}
	if o.Levels < 1 || o.Levels > 3 {
		return errors.New("DWT 级数必须在 1-3 之间")
	}
	if o.Gamma == 0 {
		o.Gamma = 1
	}
	switch o.CostStyle {
	case "", "hill", "wow", "uniward":
	default:
		return errors.New("未知成本函数: " + o.CostStyle)
	}
	return nil
}

// Normalize 校验并填充默认参数（carrier/service 层调用，幂等）。
func (o *Options) Normalize() error { return o.fillDefaults() }

// Algorithm 隐写算法统一接口。
// bits 为 0/1 位流；Extract 返回相同的 0/1 位流（长度=容量）。
type Algorithm interface {
	// ID 算法标识（与 internal/crypto 常量一致）。
	ID() byte
	// Name 算法名称。
	Name() string
	// Capacity 计算最大可嵌入位数（不含头部开销）。
	Capacity(img *image.NRGBA, opt Options) int
	// Embed 将位流嵌入图像；位流长度不得超过 Capacity。
	Embed(img *image.NRGBA, bits []byte, opt Options) error
	// Extract 从图像提取位流。
	Extract(img *image.NRGBA, opt Options) ([]byte, error)
}

// registry 算法注册表（插件化：外部可 Register 自定义算法）。
var registry = map[string]Algorithm{}

// Register 注册算法实现。
func Register(a Algorithm) {
	if a == nil {
		return
	}
	registry[a.Name()] = a
}

// Get 按名称获取算法；未注册返回 nil。
func Get(name string) Algorithm {
	return registry[name]
}

// GetByID 按 ID 获取算法。
func GetByID(id byte) Algorithm {
	for _, a := range registry {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

// Names 返回全部已注册算法名称（稳定顺序）。
func Names() []string {
	// 稳定顺序：内置算法固定在前（与历史顺序一致），其余注册算法按名称排序。
	builtin := []string{"lsb", "dct", "dwt", "hugo", "wow", "uniward"}
	out := make([]string, 0, len(registry))
	for _, n := range builtin {
		if _, ok := registry[n]; ok {
			out = append(out, n)
		}
	}
	var rest []string
	for n := range registry {
		if !knownName(n) {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// knownName 判断是否为内置算法名（Names 稳定排序使用）。
func knownName(name string) bool {
	switch name {
	case "lsb", "dct", "dwt", "hugo", "wow", "uniward":
		return true
	}
	return false
}

// ByteToBits 将字节流转为 MSB-first 0/1 位流（供 carrier/service 层使用）。
func ByteToBits(data []byte) []byte {
	bits := make([]byte, len(data)*8)
	for i, b := range data {
		for j := 0; j < 8; j++ {
			bits[i*8+j] = (b >> uint(7-j)) & 1
		}
	}
	return bits
}

// BitsToBytes 将 MSB-first 0/1 位流还原为字节（长度向下取整）。
func BitsToBytes(bits []byte) []byte {
	out := make([]byte, len(bits)/8)
	for i := range out {
		for j := 0; j < 8; j++ {
			out[i] = (out[i] << 1) | bits[i*8+j]
		}
	}
	return out
}

// errCapacity 容量不足错误。
func errCapacity(cap, need int) error {
	return fmt.Errorf("载体容量不足: 需要 %d 位, 可用 %d 位", need, cap)
}
