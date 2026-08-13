// Package ecc 提供 Reed-Solomon 纠错码与数据完整性工具。
//
// RS 在 GF(2^8) 上实现（本原多项式 0x11D），采用系统式编码：
// 每块 = 数据(255-ecc) + 冗余(ecc)，可纠正 floor(ecc/2) 个符号错误。
// 解码流程：伴随式 → Berlekamp-Massey 求错误定位多项式 → Chien 搜索 →
// Forney 求错误值并修正。
package ecc

import (
	"errors"
	"sync"
)

// fieldGen 本原多项式 x^8+x^4+x^3+x^2+1
const fieldGen = 0x11D

var (
	gfExp      [512]byte
	gfLog      [256]byte
	gfInitOnce sync.Once
)

// init 确保 GF(256) 表在包加载时初始化。
func init() { gfInit() }

// gfInit 初始化 GF(256) 指数/对数表。
func gfInit() {
	gfInitOnce.Do(func() {
		var x byte = 1
		for i := 0; i < 255; i++ {
			gfExp[i] = x
			gfLog[x] = byte(i)
			next := byte(x << 1)
			if x >= 0x80 {
				next ^= 0x1D
			}
			x = next
		}
		for i := 255; i < 512; i++ {
			gfExp[i] = gfExp[i-255]
		}
	})
}

// gfMul GF(256) 乘法。
func gfMul(x, y byte) byte {
	if x == 0 || y == 0 {
		return 0
	}
	return gfExp[int(gfLog[x])+int(gfLog[y])]
}

// gfPow 计算 x 的 power 次幂（支持负指数）。
// x = α^{log x}，故 x^power = α^{log(x)·power}。
func gfPow(x byte, power int) byte {
	if power == 0 {
		return 1
	}
	if x == 0 {
		return 0
	}
	if power < 0 {
		power = 255 + power%255
	}
	return gfExp[(int(gfLog[x])*power)%255]
}

// gfInverse 求逆元。
func gfInverse(x byte) byte {
	if x == 0 {
		return 0
	}
	return gfExp[255-int(gfLog[x])]
}

// gfPolyMul 多项式乘法（GF 系数，最高次在前）。
func gfPolyMul(p, q []byte) []byte {
	r := make([]byte, len(p)+len(q)-1)
	for i := range p {
		if p[i] == 0 {
			continue
		}
		for j := range q {
			r[i+j] ^= gfMul(p[i], q[j])
		}
	}
	return r
}

// gfPolyScale 多项式缩放。
func gfPolyScale(p []byte, x byte) []byte {
	r := make([]byte, len(p))
	for i := range p {
		r[i] = gfMul(p[i], x)
	}
	return r
}

// gfPolyAdd 多项式加法（右端对齐，常数项对齐）。
func gfPolyAdd(p, q []byte) []byte {
	if len(p) > len(q) {
		p, q = q, p
	}
	r := make([]byte, len(q))
	copy(r, q)
	off := len(q) - len(p)
	for i := range p {
		r[off+i] ^= p[i]
	}
	return r
}

// gfPolyEval 多项式求值 p(x)（最高次在前）。
func gfPolyEval(p []byte, x byte) byte {
	var y byte
	for i := 0; i < len(p); i++ {
		y = gfMul(y, x) ^ p[i]
	}
	return y
}

// gfPolyEvalLowFirst 多项式求值（低次在前，用于 Forney 的 Ω(x)）。
func gfPolyEvalLowFirst(p []byte, x byte) byte {
	var y byte
	for i := len(p) - 1; i >= 0; i-- {
		y = gfMul(y, x) ^ p[i]
	}
	return y
}

// reverseBytes 就地反转字节切片。
func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// rsGeneratorPoly 生成多项式 g(x)=Π(x-α^i), i=0..nsym-1。
func rsGeneratorPoly(nsym int) []byte {
	g := []byte{1}
	for i := 0; i < nsym; i++ {
		g = gfPolyMul(g, []byte{1, gfPow(2, i)})
	}
	return g
}

// rsEncode 系统式编码：返回 data+parity 的完整码字（255 字节）。
// 使用扩展综合除法计算校验位后，将数据区还原为原始数据（系统式）。
func rsEncode(data, gen []byte, nsym int) []byte {
	msg := make([]byte, len(data)+nsym)
	copy(msg, data)
	for i := 0; i < len(data); i++ {
		coef := msg[i]
		if coef != 0 {
			for j := 1; j < len(gen); j++ {
				msg[i+j] ^= gfMul(gen[j], coef)
			}
		}
	}
	// 还原数据区为原始数据
	copy(msg[:len(data)], data)
	return msg
}

// rsCalcSyndromes 计算伴随式 S_i = msg(α^i), i=0..nsym-1。
func rsCalcSyndromes(msg []byte, nsym int) []byte {
	synd := make([]byte, nsym)
	for i := 0; i < nsym; i++ {
		synd[i] = gfPolyEval(msg, gfPow(2, i))
	}
	return synd
}

// rsFindErrorLocator 用 Berlekamp-Massey 求错误定位多项式（最高次在前）。
func rsFindErrorLocator(synd []byte, nsym int) ([]byte, error) {
	errLoc := []byte{1}
	oldLoc := []byte{1}
	for i := 0; i < nsym; i++ {
		delta := synd[i]
		for j := 1; j < len(errLoc); j++ {
			delta ^= gfMul(errLoc[len(errLoc)-j-1], synd[i-j])
		}
		oldLoc = append(oldLoc, 0)
		if delta != 0 {
			if len(oldLoc) > len(errLoc) {
				newLoc := gfPolyScale(oldLoc, delta)
				oldLoc = gfPolyScale(errLoc, gfInverse(delta))
				errLoc = newLoc
			}
			errLoc = gfPolyAdd(errLoc, gfPolyScale(oldLoc, delta))
		}
	}
	errCount := len(errLoc) - 1
	if errCount*2 > nsym {
		return nil, errors.New("错误过多，无法纠错")
	}
	return errLoc, nil
}

// rsFindErrors 用 Chien 搜索求错误位置（从码字最左 0 起计数）。
//
// BM 输出的 errLoc 是标准定位多项式的互反形式：σ'(x)=Π(x+X_i)（低次在前，
// 根即错误定位子 X_i=α^{coefPos_i}），因此需用低次在前求值判断根，
// 命中 i 即为系数索引 coefPos_i，位置 = nmess-1-i。
func rsFindErrors(errLoc []byte, nmess int) ([]int, error) {
	errCount := len(errLoc) - 1
	if errCount == 0 {
		return nil, nil
	}
	positions := make([]int, 0, errCount)
	for i := 0; i < nmess; i++ {
		if gfPolyEvalLowFirst(errLoc, gfPow(2, i)) == 0 {
			positions = append(positions, nmess-1-i)
		}
	}
	if len(positions) != errCount {
		return nil, errors.New("错误位置定位失败（数据损坏超过纠错能力）")
	}
	return positions, nil
}

// rsFindErrorEvaluator 计算 Ω(x) = (S(x)·σ(x)) mod x^nsym，返回低次在前。
// S(x) 由伴随式构成（高次在前传入，内部反转）。
func rsFindErrorEvaluator(synd, errLoc []byte, nsym int) []byte {
	rev := append([]byte(nil), synd...)
	reverseBytes(rev)
	prod := gfPolyMul(rev, errLoc)
	var rem []byte
	if len(prod) <= nsym {
		rem = append([]byte(nil), prod...)
	} else {
		rem = append([]byte(nil), prod[len(prod)-nsym:]...)
	}
	reverseBytes(rem)
	return rem
}

// rsCorrectErrors 用 Forney 算法求错误值并修正码字。
//
// 流程（与标准 Reed-Solomon 译码一致）：
//  1. 由错误位置重建 errata 定位多项式 σ(x)=Π(α^{c}x+1)
//  2. 计算错误估值多项式 Ω(x)=(S(x)·σ(x)) mod x^{ν}
//  3. e_i = X_i^{1-ν}·Ω(X_i^{-1}) / Π_{j≠i}(1-X_j·X_i^{-1})
func rsCorrectErrors(msg, synd []byte, positions []int) ([]byte, error) {
	nmess := len(msg)
	errCount := len(positions)
	if errCount == 0 {
		return msg, nil
	}
	// coef_pos：错误位置对应的多项式系数索引（从左计数 → 最高次计数）
	coefPos := make([]int, errCount)
	for i, p := range positions {
		coefPos[i] = nmess - 1 - p
	}
	// errata 定位多项式 σ(x)=Π(α^{c}x+1)，高次在前
	errata := []byte{1}
	for _, c := range coefPos {
		errata = gfPolyMul(errata, []byte{gfPow(2, c), 1})
	}
	// Ω(x) = (S(x)·σ(x)) mod x^{ν}，低次在前
	syndRev := append([]byte(nil), synd...)
	reverseBytes(syndRev)
	prod := gfPolyMul(syndRev, errata)
	var rem []byte
	if len(prod) <= errCount {
		rem = append([]byte(nil), prod...)
	} else {
		rem = append([]byte(nil), prod[len(prod)-errCount:]...)
	}
	reverseBytes(rem)

	// X_j = α^{coef_pos_j}
	X := make([]byte, errCount)
	XInv := make([]byte, errCount)
	for i, c := range coefPos {
		X[i] = gfPow(2, c)
		XInv[i] = gfInverse(X[i])
	}

	for i, pos := range positions {
		XiInv := XInv[i]
		// err_loc_prime = Π_{j≠i}(1 - X_j·X_i^{-1})
		errLocPrime := byte(1)
		for j := range X {
			if j == i {
				continue
			}
			errLocPrime = gfMul(errLocPrime, 1^gfMul(XiInv, X[j]))
		}
		if errLocPrime == 0 {
			return nil, errors.New("错误值计算失败（不可纠）")
		}
		// magnitude = Ω(X_i^{-1}) / err_loc_prime
		// 说明：本实现的 Ω 取尾部 ν 项（x^{nsym}·W(x) 的高次部分），代入后
		// Ω(X_i^{-1}) 已含 X_i^{1-ν} 因子，与 err_loc_prime（同样含 X_i^{1-ν}）相除恰好约掉。
		omegaVal := gfPolyEvalLowFirst(rem, XiInv)
		magnitude := gfMul(omegaVal, gfInverse(errLocPrime))
		msg[pos] ^= magnitude
	}
	return msg, nil
}

// rsCorrectBlock 单块纠错（data+parity 完整码字）。
// 内部拷贝后修正，不修改调用方数据。
func rsCorrectBlock(msg []byte, nsym int) ([]byte, error) {
	work := append([]byte(nil), msg...)
	synd := rsCalcSyndromes(work, nsym)
	hasErr := false
	for _, s := range synd {
		if s != 0 {
			hasErr = true
			break
		}
	}
	if !hasErr {
		return work, nil
	}
	errLoc, err := rsFindErrorLocator(synd, nsym)
	if err != nil {
		return nil, err
	}
	positions, err := rsFindErrors(errLoc, len(work))
	if err != nil {
		return nil, err
	}
	return rsCorrectErrors(work, synd, positions)
}
