package algorithm

import (
	"errors"
	"image"
)

// lsb 抗检测伪随机游走 LSB 算法。
//
// 升级点（相对 V1.0 pkg/steg）：
//  1. 通道掩码：可指定只在 R/G/B 部分通道嵌入
//  2. 多深度：每通道 1-4 bit，深度越高容量越大
//  3. 数据耗尽后剩余位填充确定性随机噪声，抹平统计偏差
//  4. 游走序列由 seed 派生，跨平台一致
type lsb struct{}

// NewLSB 创建 LSB 算法实例。
func NewLSB() Algorithm { return &lsb{} }

func (a *lsb) ID() byte   { return IDLSB }
func (a *lsb) Name() string { return "lsb" }

// Capacity 返回最大可嵌入位数。
func (a *lsb) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	if err := opt.fillDefaults(); err != nil {
		return 0
	}
	b := img.Bounds()
	channels := popCount(opt.ChannelMask)
	return b.Dx() * b.Dy() * channels * opt.BitDepth
}

// Embed 将位流写入图像（伪随机游走）。
func (a *lsb) Embed(img *image.NRGBA, bits []byte, opt Options) error {
	if img == nil {
		return errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return err
	}
	cap := a.Capacity(img, opt)
	if len(bits) > cap {
		return errCapacity(cap, len(bits))
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cursor := NewPixelCursor(opt.Seed, w, h)
	pos := 0

	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		ch := &c
		// 按掩码顺序写入 R,G,B（各通道低 bitDepth 位）
		if opt.ChannelMask&1 != 0 {
			ch.R = writeChannelBits(ch.R, opt.BitDepth, func() byte { return nextBit(bits, &pos, cursor.rng) })
		}
		if opt.ChannelMask&2 != 0 {
			ch.G = writeChannelBits(ch.G, opt.BitDepth, func() byte { return nextBit(bits, &pos, cursor.rng) })
		}
		if opt.ChannelMask&4 != 0 {
			ch.B = writeChannelBits(ch.B, opt.BitDepth, func() byte { return nextBit(bits, &pos, cursor.rng) })
		}
		img.SetNRGBA(x, y, c)
	}
	return nil
}

// Extract 按相同游走序列读取全部位流（含噪声区，长度=Capacity）。
func (a *lsb) Extract(img *image.NRGBA, opt Options) ([]byte, error) {
	if img == nil {
		return nil, errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cursor := NewPixelCursor(opt.Seed, w, h)
	stream := make([]byte, 0, a.Capacity(img, opt))

	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		if opt.ChannelMask&1 != 0 {
			stream = append(stream, readChannelBits(c.R, opt.BitDepth)...)
		}
		if opt.ChannelMask&2 != 0 {
			stream = append(stream, readChannelBits(c.G, opt.BitDepth)...)
		}
		if opt.ChannelMask&4 != 0 {
			stream = append(stream, readChannelBits(c.B, opt.BitDepth)...)
		}
	}
	return stream, nil
}

// nextBit 依次取数据位，耗尽后用随机噪声位填充。
func nextBit(stream []byte, pos *int, rng *Xorshift64Star) byte {
	if *pos < len(stream) {
		b := stream[*pos]
		*pos++
		return b
	}
	return rng.NextBit()
}

// writeChannelBits 将 bitDepth 个 bit 写入通道低 bitDepth 位（bit0=LSB）。
func writeChannelBits(v uint8, bitDepth int, next func() byte) uint8 {
	for i := 0; i < bitDepth; i++ {
		mask := uint8(1 << uint(i))
		if next() == 1 {
			v |= mask
		} else {
			v &^= mask
		}
	}
	return v
}

// readChannelBits 从通道低 bitDepth 位读取 bit 序列（bit0=LSB 在前）。
func readChannelBits(v uint8, bitDepth int) []byte {
	out := make([]byte, bitDepth)
	for i := 0; i < bitDepth; i++ {
		out[i] = (v >> uint(i)) & 1
	}
	return out
}

// popCount 统计掩码中置位个数。
func popCount(mask int) int {
	n := 0
	for mask != 0 {
		n += mask & 1
		mask >>= 1
	}
	return n
}
