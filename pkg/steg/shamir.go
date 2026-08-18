package steg

import (
	"errors"
	"math/rand"
)

// =============================================================
// Shamir 门限分片（Shamir's Secret Sharing）
//
// 在 GF(2^8) 有限域上实现 (k, n) 门限：n 个分片任意凑齐 k 个即可完整恢复，
// 少于 k 个分片得不到任何明文信息。可用于将秘密分发到多张载体。
// =============================================================

const (
	fieldSize = 256
	generator = 3
)

var (
	gfLog      [fieldSize]uint8
	gfExp      [fieldSize*2 - 1]uint8
	gfInitDone bool
)

func gfInit() {
	if gfInitDone {
		return
	}
	x := uint8(1)
	for i := 0; i < fieldSize-1; i++ {
		gfExp[i] = x
		gfLog[x] = uint8(i)
		x = gfMulTable(x, generator)
	}
	for i := fieldSize - 1; i < len(gfExp); i++ {
		gfExp[i] = gfExp[i-(fieldSize-1)]
	}
	gfInitDone = true
}

// gfMulTable 基础乘法（表构建用）。
func gfMulTable(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	var res uint16
	aa := uint16(a)
	for b != 0 {
		if b&1 != 0 {
			res ^= aa
		}
		aa <<= 1
		if aa&0x100 != 0 {
			aa ^= 0x11B // AES 不可约多项式 x^8+x^4+x^3+x+1
		}
		b >>= 1
	}
	return uint8(res)
}

// gfMul GF(2^8) 乘法。
func gfMul(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// gfDiv GF(2^8) 除法。
func gfDiv(a, b uint8) (uint8, error) {
	if b == 0 {
		return 0, errors.New("GF(256) 除数为零")
	}
	if a == 0 {
		return 0, nil
	}
	return gfExp[(int(gfLog[a])-int(gfLog[b])+fieldSize-1)%(fieldSize-1)], nil
}

// gfEvalPoly 用 Horner 规则在 x 处求多项式值。
func gfEvalPoly(coeffs []uint8, x uint8) uint8 {
	if x == 0 {
		if len(coeffs) == 0 {
			return 0
		}
		return coeffs[0]
	}
	var result uint8
	for i := len(coeffs) - 1; i >= 0; i-- {
		result = gfMul(result, x) ^ coeffs[i]
	}
	return result
}

// shareHeaderLen 每个分片携带的 x 坐标头长度。
// 分片格式：[x byte] + [y 数据...]，其中 x = 分片编号(1-based)。
const shareHeaderLen = 1

// SplitSecret 将数据拆分为 (total, threshold) 门限分片。
// 返回 total 个分片，任意凑齐 threshold 个即可恢复（分片内嵌 x 坐标，与选取顺序无关）。
func SplitSecret(data []byte, total, threshold int) ([][]byte, error) {
	gfInit()
	if len(data) == 0 {
		return nil, errors.New("秘密数据为空")
	}
	if threshold <= 0 || threshold > total {
		return nil, errors.New("门限需满足 1 ≤ threshold ≤ total")
	}
	if total > fieldSize-1 {
		return nil, errors.New("分片数量不能超过 255")
	}
	if threshold == 1 {
		// 门限为 1：每个分片直接携带完整数据
		shares := make([][]byte, total)
		for i := range shares {
			shares[i] = make([]byte, 0, len(data)+shareHeaderLen)
			shares[i] = append(shares[i], uint8(i+1))
			shares[i] = append(shares[i], data...)
		}
		return shares, nil
	}

	shares := make([][]byte, total)
	for i := range shares {
		shares[i] = make([]byte, len(data)+shareHeaderLen)
		shares[i][0] = uint8(i + 1)
	}

	rng := rand.New(rand.NewSource(0x5EEDC0DE)) // 种子固定不用于安全，系数随机性由后续加密保证
	for i := 0; i < len(data); i++ {
		coeffs := make([]uint8, threshold)
		coeffs[0] = data[i]
		for c := 1; c < threshold; c++ {
			coeffs[c] = uint8(rng.Intn(fieldSize))
		}
		for s := 0; s < total; s++ {
			shares[s][i+shareHeaderLen] = gfEvalPoly(coeffs, uint8(s+1))
		}
	}
	return shares, nil
}

// RecoverSecret 用至少 threshold 个分片恢复数据。
// 分片可任意选取/任意顺序；每个分片的 x 坐标由其首字节标识。
func RecoverSecret(shares [][]byte, threshold int) ([]byte, error) {
	gfInit()
	if len(shares) == 0 {
		return nil, errors.New("无分片数据")
	}
	if len(shares) < threshold {
		return nil, errors.New("分片数量不足，无法恢复")
	}
	length := len(shares[0])
	if length < shareHeaderLen+1 {
		return nil, errors.New("分片数据非法（缺少 x 坐标头）")
	}
	for _, s := range shares {
		if len(s) != length {
			return nil, errors.New("分片长度不一致")
		}
	}

	used := shares[:threshold]
	xs := make([]uint8, threshold)
	for j, s := range used {
		xs[j] = s[0]
		for m := 0; m < j; m++ {
			if xs[m] == xs[j] {
				return nil, errors.New("存在 x 坐标重复的分片（请勿重复选择同一分片）")
			}
		}
	}

	dataLen := length - shareHeaderLen
	out := make([]byte, dataLen)
	for i := 0; i < dataLen; i++ {
		var secret uint8
		for j := 0; j < threshold; j++ {
			// Lagrange 基多项式在 0 处的值：L_j(0) = Π_{m≠j} x_m / (x_m - x_j)
			num := uint8(1)
			den := uint8(1)
			for m := 0; m < threshold; m++ {
				if m == j {
					continue
				}
				num = gfMul(num, xs[m])
				den = gfMul(den, xs[m]^xs[j])
			}
			lj0, err := gfDiv(num, den)
			if err != nil {
				return nil, err
			}
			secret ^= gfMul(used[j][i+shareHeaderLen], lj0)
		}
		out[i] = secret
	}
	return out, nil
}
