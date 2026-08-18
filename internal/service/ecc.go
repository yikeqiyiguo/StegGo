// 隐写链路可选 Reed-Solomon 容错编码（ECC）。
//
// 原理：对完整 V3 加密载荷再做 RS 纠错编码，整体嵌入载体。
// 载体在"社交压缩"（微信/QQ/邮件重压缩）、局部损坏时出现少量位翻转，
// 提取阶段先 RS 纠错恢复原始载荷，再走魔数定位与解密，显著提高抗损鲁棒性。
//
// 包装布局（嵌入流的最前部，紧接载体提取流的头部）：
//
//	[魔数 "STECC" 5B][level 1B][encLen 4B BE][Reed-Solomon 编码数据]
//
// 魔数固定 5 字节，误命中概率 2^-40 可忽略；未启用 ECC 的老载体不含该头，
// unwrapECC 快速返回 (ok=false)，链路完全向后兼容。
package service

import (
	"bytes"
	"encoding/binary"
	"errors"

	"steggo/internal/crypto/ecc"
)

// eccMagic ECC 包装头魔数（提取时用于识别）。
var eccMagic = []byte("STECC")

// eccHeaderLen ECC 包装头长度 = 魔数 5 + level 1 + encLen 4。
const eccHeaderLen = 10

// eccLevel 将 CLI 等级字符串映射为纠错等级。
func eccLevel(level string) (ecc.Level, error) {
	switch level {
	case "low":
		return ecc.LevelLow, nil
	case "", "medium":
		return ecc.LevelMedium, nil
	case "high":
		return ecc.LevelHigh, nil
	default:
		return 0, errors.New("ECC 等级必须为 low|medium|high")
	}
}

// eccLevelName 纠错等级的可读名。
func eccLevelName(lv ecc.Level) string {
	switch lv {
	case ecc.LevelLow:
		return "low"
	case ecc.LevelHigh:
		return "high"
	default:
		return "medium"
	}
}

// wrapECC 对载荷做 RS 编码并加包装头。
func wrapECC(payload []byte, level string) ([]byte, *ecc.Stats, error) {
	lv, err := eccLevel(level)
	if err != nil {
		return nil, nil, err
	}
	encoded, stats, err := ecc.Encode(payload, lv)
	if err != nil {
		return nil, nil, err
	}
	out := make([]byte, 0, eccHeaderLen+len(encoded))
	out = append(out, eccMagic...)
	out = append(out, byte(lv))
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(encoded)))
	out = append(out, lb[:]...)
	out = append(out, encoded...)
	return out, stats, nil
}

// unwrapECC 检测并解码 ECC 包装流。
// 返回 (解码后载荷, 纠错等级, 统计, 是否命中 ECC 头, 错误)。
// 未命中 ECC 头时 ok=false 且不返回错误（向后兼容老载体）。
func unwrapECC(stream []byte) ([]byte, string, *ecc.Stats, bool, error) {
	idx := bytes.Index(stream, eccMagic)
	if idx < 0 || idx+eccHeaderLen > len(stream) {
		return nil, "", nil, false, nil
	}
	lv := ecc.Level(stream[idx+len(eccMagic)])
	if lv != ecc.LevelLow && lv != ecc.LevelMedium && lv != ecc.LevelHigh {
		return nil, "", nil, false, nil
	}
	encLen := int(binary.BigEndian.Uint32(stream[idx+len(eccMagic)+1 : idx+eccHeaderLen]))
	if encLen <= 0 || idx+eccHeaderLen+encLen > len(stream) {
		return nil, "", nil, false, nil
	}
	data, stats, err := ecc.Decode(stream[idx+eccHeaderLen:idx+eccHeaderLen+encLen], lv)
	if err != nil {
		return nil, "", stats, true, err
	}
	return data, eccLevelName(lv), stats, true, nil
}
