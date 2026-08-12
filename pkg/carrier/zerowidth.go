package carrier

import (
	"encoding/binary"
	"errors"
	"strings"
)

// =============================================================
// 零宽字符隐写（文本类载体 TXT/MD）
//
// 编码表：
//
//	U+200B ZERO WIDTH SPACE   → bit 0
//	U+200C ZERO WIDTH NON-JOINER → bit 1
//
// 载荷布局：[magic "STGZW2"][len uint32 BE][data bits]
// 零宽字符对肉眼完全不可见，文本渲染零感知。
// =============================================================

const (
	zeroWidthMagic = "STGZW2"
	zwSpace        = '\u200b'
	zwNonJoiner    = '\u200c'
)

// EncodeZeroWidth 将数据编码为不可见零宽字符，追加到文本末尾。
func EncodeZeroWidth(text string, data []byte) string {
	var sb strings.Builder
	for i := 0; i < len(zeroWidthMagic); i++ {
		writeZWBits(&sb, zeroWidthMagic[i])
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	for i := 0; i < 4; i++ {
		writeZWBits(&sb, lenBuf[i])
	}
	for _, b := range data {
		writeZWBits(&sb, b)
	}
	return text + sb.String()
}

func writeZWBits(sb *strings.Builder, b byte) {
	for i := 7; i >= 0; i-- {
		if b&(1<<uint(i)) != 0 {
			sb.WriteRune(zwNonJoiner)
		} else {
			sb.WriteRune(zwSpace)
		}
	}
}

// DecodeZeroWidth 从文本中提取零宽字符隐写数据。
func DecodeZeroWidth(text string) ([]byte, error) {
	// 按连续零宽段扫描
	var bits []byte
	flush := func() ([]byte, bool) {
		if len(bits) == 0 {
			return nil, false
		}
		data, ok := tryParseZWBits(bits)
		bits = bits[:0]
		return data, ok
	}
	for _, r := range text {
		switch r {
		case zwSpace:
			bits = append(bits, 0)
		case zwNonJoiner:
			bits = append(bits, 1)
		default:
			if data, ok := flush(); ok {
				return data, nil
			}
		}
	}
	if data, ok := flush(); ok {
		return data, nil
	}
	return nil, errors.New("文本中未找到 StegGo 零宽字符载荷")
}

func tryParseZWBits(bits []byte) ([]byte, bool) {
	if len(bits) < (len(zeroWidthMagic)+4)*8 {
		return nil, false
	}
	bs := bitsToBytes(bits)
	if string(bs[:len(zeroWidthMagic)]) != zeroWidthMagic {
		return nil, false
	}
	dataLen := int(binary.BigEndian.Uint32(bs[len(zeroWidthMagic) : len(zeroWidthMagic)+4]))
	need := len(zeroWidthMagic) + 4 + dataLen
	if len(bs) < need {
		return nil, false
	}
	out := make([]byte, dataLen)
	copy(out, bs[len(zeroWidthMagic)+4:need])
	return out, true
}

func bitsToBytes(bits []byte) []byte {
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

// ContainsZeroWidth 判断文本中是否含零宽字符。
func ContainsZeroWidth(text string) bool {
	return strings.ContainsRune(text, zwSpace) || strings.ContainsRune(text, zwNonJoiner)
}
