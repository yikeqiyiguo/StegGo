package ecc

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// Level 纠错强度等级。
type Level uint8

// 纠错等级：冗余符号数/块。
const (
	LevelLow    Level = 4  // 冗余 ~1.6%，可纠 2 字节/块
	LevelMedium Level = 16 // 冗余 ~6.3%，可纠 8 字节/块
	LevelHigh   Level = 32 // 冗余 ~12.5%，可纠 16 字节/块
)

// MaxDataLen 每块最大数据字节数。
const MaxDataLen = 255 - 32 // 255 - 最大冗余

// Stats 描述一次纠错编码/解码的统计信息。
type Stats struct {
	OriginalLen    int     // 原始数据长度
	EncodedLen     int     // 编码后长度
	RedundancyRatio float64 // 冗余占比 = 冗余/总长
	Blocks         int     // 数据块数
	CorrectedErrors int    // 解码时纠正的符号总数
	RepairRate     float64 // 修复成功率预估（修复块数/总块数）
}

// LevelECC 返回等级对应的冗余符号数。
func (l Level) eccBytes() int {
	switch l {
	case LevelLow:
		return 4
	case LevelHigh:
		return 32
	default:
		return 16
	}
}

// headerSize 编码数据头的长度（原始长度 4B）。
const headerSize = 4

// Encode 对数据应用 Reed-Solomon 纠错编码。
// 输出布局：[origLen 4B BE][块1][块2]...，每块 = data(ecc 相关) + parity。
func Encode(data []byte, level Level) ([]byte, *Stats, error) {
	if level == 0 {
		level = LevelMedium
	}
	eccN := level.eccBytes()
	dataLen := 255 - eccN
	if dataLen <= 0 {
		return nil, nil, errors.New("纠错参数非法")
	}
	g := rsGeneratorPoly(eccN)

	blocks := (len(data) + dataLen - 1) / dataLen
	if blocks == 0 {
		blocks = 1
	}
	out := make([]byte, headerSize+blocks*255)
	binary.BigEndian.PutUint32(out[:4], uint32(len(data)))

	off := headerSize
	for i := 0; i < blocks; i++ {
		start := i * dataLen
		end := start + dataLen
		if end > len(data) {
			end = len(data)
		}
		blockData := make([]byte, dataLen)
		copy(blockData, data[start:end])
		code := rsEncode(blockData, g, eccN)
		copy(out[off:], code)
		off += 255
	}
	stats := &Stats{
		OriginalLen:     len(data),
		EncodedLen:      len(out),
		RedundancyRatio: float64(blocks*eccN) / float64(len(out)),
		Blocks:          blocks,
	}
	return out, stats, nil
}

// Decode 解码并纠错。
func Decode(encoded []byte, level Level) ([]byte, *Stats, error) {
	if level == 0 {
		level = LevelMedium
	}
	eccN := level.eccBytes()
	dataLen := 255 - eccN
	if len(encoded) < headerSize {
		return nil, nil, errors.New("编码数据过短")
	}
	origLen := int(binary.BigEndian.Uint32(encoded[:4]))
	body := encoded[headerSize:]
	if len(body) == 0 || len(body)%255 != 0 {
		return nil, nil, errors.New("编码数据长度非法")
	}
	blocks := len(body) / 255
	corrected := 0
	repaired := 0
	out := make([]byte, 0, blocks*dataLen)
	for i := 0; i < blocks; i++ {
		code := append([]byte(nil), body[i*255:(i+1)*255]...)
		fixed, err := rsCorrectBlock(code, eccN)
		if err != nil {
			// 该块不可纠：使用原始块，统计为失败
			fixed = code
		} else {
			repaired++
		}
		// 统计纠正符号数：比较与原始块差异
		for j := 0; j < 255; j++ {
			if fixed[j] != code[j] {
				corrected++
			}
		}
		out = append(out, fixed[:dataLen]...)
	}
	if origLen > len(out) {
		return nil, nil, errors.New("解码结果异常")
	}
	// 修复率 = 成功块/总块
	repairRate := 0.0
	if blocks > 0 {
		repairRate = float64(repaired) / float64(blocks)
	}
	stats := &Stats{
		OriginalLen:      origLen,
		EncodedLen:       len(encoded),
		RedundancyRatio:  float64(blocks*eccN) / float64(len(encoded)),
		Blocks:           blocks,
		CorrectedErrors:  corrected,
		RepairRate:       repairRate,
	}
	return out[:origLen], stats, nil
}

// CRC 相关工具（MP4 视频帧独立校验）。
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// FrameCRC 计算帧数据 CRC32（Castagnoli）。
func FrameCRC(data []byte) uint32 {
	return crc32.Checksum(data, crcTable)
}

// Frame 描述一个视频帧（数据 + 独立 CRC）。
type Frame struct {
	Data []byte
	CRC  uint32
}

// TagFrames 为每帧附加 CRC 标签，返回带标签的帧序列。
// 输出布局：[count 4B][每帧: len 4B][crc 4B][data]。
func TagFrames(frames [][]byte) ([]byte, error) {
	var out []byte
	var cnt [4]byte
	binary.BigEndian.PutUint32(cnt[:], uint32(len(frames)))
	out = append(out, cnt[:]...)
	for _, f := range frames {
		var lb [4]byte
		binary.BigEndian.PutUint32(lb[:], uint32(len(f)))
		out = append(out, lb[:]...)
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], FrameCRC(f))
		out = append(out, cb[:]...)
		out = append(out, f...)
	}
	return out, nil
}

// RepairFrames 按帧 CRC 校验，自动跳过损坏帧并重组数据。
// 返回 (重组数据, 有效帧数, 总帧数, 修复率)。
func RepairFrames(tagged []byte) ([]byte, int, int, float64, error) {
	if len(tagged) < 4 {
		return nil, 0, 0, 0, errors.New("帧序列数据过短")
	}
	count := int(binary.BigEndian.Uint32(tagged[:4]))
	if count <= 0 || count > 100000 {
		return nil, 0, 0, 0, errors.New("帧数量非法")
	}
	var out []byte
	valid := 0
	off := 4
	for i := 0; i < count; i++ {
		if off+8 > len(tagged) {
			return nil, valid, count, 0, errors.New("帧数据不完整")
		}
		flen := int(binary.BigEndian.Uint32(tagged[off : off+4]))
		fcrc := binary.BigEndian.Uint32(tagged[off+4 : off+8])
		off += 8
		if flen < 0 || off+flen > len(tagged) {
			return nil, valid, count, 0, errors.New("帧数据不完整")
		}
		frame := tagged[off : off+flen]
		off += flen
		if FrameCRC(frame) == fcrc {
			out = append(out, frame...)
			valid++
		}
		// CRC 不匹配：跳过损坏帧
	}
	rate := 0.0
	if count > 0 {
		rate = float64(valid) / float64(count)
	}
	return out, valid, count, rate, nil
}
