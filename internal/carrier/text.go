package carrier

import (
	"encoding/binary"
	"os"
	"strings"
)

// 零宽字符编码：U+200B (ZWSP) 表示 0，U+200D (ZWJ) 表示 1。
// 文本载体将数据以零宽字符序列写入文件开头，肉眼不可见，不影响原文阅读。
const (
	zwZero = "\u200b" // ZERO WIDTH SPACE
	zwOne  = "\u200d" // ZERO WIDTH JOINER
)

// textCarrier 文本载体（TXT/MD 零宽字符隐藏）。
type textCarrier struct{}

func (c *textCarrier) Kind() Kind { return KindText }

func (c *textCarrier) Extensions() []string { return []string{".txt", ".md", ".markdown"} }

// Capacity 理论容量取决于文本长度（每个字节 8 个零宽字符）。
// 保守估计：文本总字符数 / 16（一半为可见文本）。
func (c *textCarrier) Capacity(path string, opt Options) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return int64(len(data) / 16), nil
}

func (c *textCarrier) HasCapacity(path string, size int64, opt Options) (bool, error) {
	cap, err := c.Capacity(path, opt)
	if err != nil {
		return false, err
	}
	return size <= cap, nil
}

// Embed 在文本文件开头插入零宽编码：[8B 长度头][payload]。
func (c *textCarrier) Embed(path, outPath string, payload []byte, opt Options) error {
	if len(payload) > maxTailPayload {
		return ErrTooLarge
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var hdr [8]byte
	binary.BigEndian.PutUint64(hdr[:], uint64(len(payload)))
	var sb strings.Builder
	sb.Grow(len(payload)*8 + 64 + len(original))
	sb.WriteString(zwEncode(hdr[:]))
	sb.WriteString(zwEncode(payload))
	sb.Write(original)
	return os.WriteFile(outPath, []byte(sb.String()), 0644)
}

// Extract 从文本开头收集连续零宽字符序列并解码。
func (c *textCarrier) Extract(path string, opt Options) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seq := collectZeroWidthRun(string(data))
	if len(seq) < 64 {
		return nil, ErrNoPayload
	}
	decoded := zwDecode(seq)
	payloadLen := int64(binary.BigEndian.Uint64(decoded[:8]))
	if payloadLen <= 0 || payloadLen > maxTailPayload {
		return nil, ErrCorrupted
	}
	if int64(len(decoded)) < 8+payloadLen {
		return nil, ErrCorrupted
	}
	return decoded[8 : 8+payloadLen], nil
}

// zwEncode 将字节流编码为 MSB-first 零宽字符序列。
func zwEncode(data []byte) string {
	var sb strings.Builder
	sb.Grow(len(data) * 8)
	for _, b := range data {
		for j := 7; j >= 0; j-- {
			if b&(1<<uint(j)) != 0 {
				sb.WriteString(zwOne)
			} else {
				sb.WriteString(zwZero)
			}
		}
	}
	return sb.String()
}

// collectZeroWidthRun 收集字符串开头的首个连续零宽字符段。
// 返回该段内的 0/1 位序列（每元素 0 或 1）。
func collectZeroWidthRun(s string) []byte {
	var bits []byte
	for _, r := range s {
		switch string(r) {
		case zwZero:
			bits = append(bits, 0)
		case zwOne:
			bits = append(bits, 1)
		default:
			if len(bits) == 0 {
				continue // 跳过可见字符，直到首个零宽段
			}
			return bits // 首个连续段结束
		}
	}
	return bits
}

// zwDecode 将 0/1 位序列还原为字节（MSB-first，向下取整）。
func zwDecode(bits []byte) []byte {
	if len(bits) < 8 {
		return nil
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		for j := 0; j < 8; j++ {
			if bits[i*8+j] == 1 {
				out[i] |= 1 << uint(7-j)
			}
		}
	}
	return out
}
