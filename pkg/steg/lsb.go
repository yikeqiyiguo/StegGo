package steg

import (
	"errors"
	"image"
	"image/color"
)

// =============================================================
// 自研抗检测 LSB 隐写算法（核心壁垒）
//
// 传统顺序 LSB 特征明显、极易被工具扫描。本算法：
//  1. 由 密码+固定盐 → PBKDF2 → 随机种子 → 伪随机像素坐标序列（不按顺序遍历像素）
//  2. 数据按位轮循写入 R/G/B 三通道，避免单通道特征聚集
//  3. 有效数据写完后，空余 LSB 位填充高斯随机噪声
//  4. 使整个 LSB 平面统计均匀，对抗卡方检验 / RS分析 / SPA 检测
// =============================================================

// EmbedLSB 将位流写入 NRGBA 图像。
// stream 为 0/1 位数组；bitDepth ∈ [1,4] 为每通道嵌入位数。
// 数据位耗尽后，剩余所有 LSB 位用确定性随机噪声填充（抹平统计偏差）。
func EmbedLSB(img *image.NRGBA, stream []byte, seed []byte, bitDepth int) error {
	if img == nil {
		return errors.New("图像为空")
	}
	if bitDepth < 1 || bitDepth > 4 {
		return errors.New("嵌入位数必须在 1-4 之间")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return errors.New("图像尺寸非法")
	}

	cursor := NewPixelCursor(seed, w, h)
	pos := 0

	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		// R/G/B 三通道轮循，每通道写 bitDepth 位
		c.R = writeChannelBits(c.R, bitDepth, func() byte {
			return nextBit(stream, &pos, cursor.rng)
		})
		c.G = writeChannelBits(c.G, bitDepth, func() byte {
			return nextBit(stream, &pos, cursor.rng)
		})
		c.B = writeChannelBits(c.B, bitDepth, func() byte {
			return nextBit(stream, &pos, cursor.rng)
		})
		img.SetNRGBA(x, y, c)
	}
	return nil
}

// ExtractLSB 按与 EmbedLSB 相同的随机坐标序列读取全部位流（含噪声区）。
// 返回 0/1 位数组，长度为 w*h*3*bitDepth。
func ExtractLSB(img *image.NRGBA, seed []byte, bitDepth int) ([]byte, error) {
	if img == nil {
		return nil, errors.New("图像为空")
	}
	if bitDepth < 1 || bitDepth > 4 {
		return nil, errors.New("嵌入位数必须在 1-4 之间")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	cursor := NewPixelCursor(seed, w, h)
	totalBits := w * h * 3 * bitDepth
	stream := make([]byte, 0, totalBits)

	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		stream = append(stream, readChannelBits(c.R, bitDepth)...)
		stream = append(stream, readChannelBits(c.G, bitDepth)...)
		stream = append(stream, readChannelBits(c.B, bitDepth)...)
	}
	if len(stream) < totalBits {
		return nil, errors.New("读取位流不完整")
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

// ByteToBits 将字节流转换为 MSB-first 位数组。
func ByteToBits(data []byte) []byte {
	bits := make([]byte, len(data)*8)
	for i, b := range data {
		for j := 0; j < 8; j++ {
			bits[i*8+j] = (b >> uint(7-j)) & 1
		}
	}
	return bits
}

// BitsToBytes 将 MSB-first 位数组转换为字节流。
func BitsToBytes(bits []byte) []byte {
	out := make([]byte, len(bits)/8)
	for i := range out {
		var v byte
		for j := 0; j < 8; j++ {
			v = v<<1 | (bits[i*8+j] & 1)
		}
		out[i] = v
	}
	return out
}

// CapLSBBytes 计算图像在给定嵌入位数下的最大可容纳字节数。
func CapLSBBytes(img *image.NRGBA, bitDepth int) int {
	b := img.Bounds()
	return (b.Dx() * b.Dy() * 3 * bitDepth) / 8
}

// clamp 保持 color 接口兼容（避免误用）。
func clamp(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

var _ color.Color = color.NRGBA{}
