package service

import (
	"bytes"

	"steggo/internal/carrier"
	"steggo/internal/common"
)

// combo 一次算法参数尝试组合。
type combo struct {
	algorithm string
	bitDepth  int
	quality   int
	levels    int
	costStyle string
}

// scanCombos 提取时按优先级扫描的算法参数矩阵。
// 覆盖 V2.0 全部内置算法及其常用参数；用户显式指定算法时优先尝试。
var scanCombos = []combo{
	// LSB 深度从高到低（V1.0 默认深度 2，历史优先级靠前）
	{"lsb", 2, 0, 0, ""},
	{"lsb", 1, 0, 0, ""},
	{"lsb", 3, 0, 0, ""},
	{"lsb", 4, 0, 0, ""},
	// DCT 常用质量
	{"dct", 0, 8, 0, ""},
	{"dct", 0, 16, 0, ""},
	// DWT 各级数
	{"dwt", 0, 0, 2, ""},
	{"dwt", 0, 0, 1, ""},
	{"dwt", 0, 0, 3, ""},
	// 自适应
	{"hugo", 0, 0, 0, "hill"},
	{"wow", 0, 0, 0, "wow"},
	{"uniward", 0, 0, 0, "uniward"},
}

// scanImageExtract 对图像载体扫描算法矩阵提取载荷。
// 找到 V3（或 V2）魔数即命中，返回位流还原后的字节流。
func scanImageExtract(path string, opt Options, seed []byte) ([]byte, string, int, error) {
	c := carrier.Get(carrier.KindImage)
	if c == nil {
		return nil, "", 0, carrier.ErrUnsupportedFormat
	}

	// 用户显式指定的算法优先尝试一次。
	combos := make([]combo, 0, len(scanCombos)+1)
	if opt.Algorithm != "" && opt.Algorithm != "lsb" {
		combos = append(combos, combo{
			algorithm: opt.Algorithm,
			bitDepth:  opt.BitDepth,
			quality:   opt.Quality,
			levels:    opt.Levels,
			costStyle: opt.CostStyle,
		})
	}
	combos = append(combos, scanCombos...)

	var lastErr error
	for _, cm := range combos {
		copt := opt.carrierOptions(seed)
		copt.Algorithm = cm.algorithm
		if cm.bitDepth > 0 {
			copt.BitDepth = cm.bitDepth
		}
		if cm.quality > 0 {
			copt.Quality = cm.quality
		}
		if cm.levels > 0 {
			copt.Levels = cm.levels
		}
		if cm.costStyle != "" {
			copt.CostStyle = cm.costStyle
		}
		stream, err := c.Extract(path, copt)
		if err != nil {
			lastErr = err
			continue
		}
		if len(stream) >= len(common.MagicV3) &&
			bytes.HasPrefix(stream, []byte(common.MagicV3)) {
			return stream, cm.algorithm, cm.bitDepth, nil
		}
		if len(stream) >= len(common.MagicV2) &&
			bytes.HasPrefix(stream, []byte(common.MagicV2)) {
			// V2 魔数命中的是 V1.0 旧载体，交给 service.Extract 的 V1 兼容回退处理
			return stream, "lsb", cm.bitDepth, nil
		}
	}
	if lastErr == nil {
		lastErr = carrier.ErrNoPayload
	}
	return nil, "", 0, lastErr
}
