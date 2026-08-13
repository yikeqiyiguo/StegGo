package algorithm

import (
	"image"
	"math"
)

// =============================================================
// 载体分析：容量 / PSNR / SSIM / 卡方检验 / RS 分析
// 用于嵌入前评估载体安全性与嵌入后质量评估。
// =============================================================

// CapacityBytes 计算算法在给定深度下的最大容量（字节）。
func CapacityBytes(algo Algorithm, img *image.NRGBA, opt Options) int {
	if algo == nil {
		return 0
	}
	return algo.Capacity(img, opt) / 8
}

// PSNR 计算峰值信噪比（dB）。两张图尺寸必须一致。
// 0 表示完全相同（返回 Inf）。
func PSNR(a, b *image.NRGBA) float64 {
	if a == nil || b == nil {
		return 0
	}
	ba, bb := a.Bounds(), b.Bounds()
	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return 0
	}
	var mse float64
	count := 0
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			ca, cb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			mse += sq(float64(ca.R)-float64(cb.R)) +
				sq(float64(ca.G)-float64(cb.G)) +
				sq(float64(ca.B)-float64(cb.B))
			count += 3
		}
	}
	if count == 0 {
		return 0
	}
	mse /= float64(count)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

// SSIM 计算结构相似性（简化：全图均值/方差，L=255，K1=0.01，K2=0.03）。
// 返回 [0,1]，越接近 1 越相似。
func SSIM(a, b *image.NRGBA) float64 {
	if a == nil || b == nil {
		return 0
	}
	ba, bb := a.Bounds(), b.Bounds()
	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return 0
	}
	const c1 = (0.01 * 255) * (0.01 * 255)
	const c2 = (0.03 * 255) * (0.03 * 255)

	// 灰度均值
	var sa, sb float64
	n := 0
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			sa += grayVal(a, x, y)
			sb += grayVal(b, x, y)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	ua, ub := sa/float64(n), sb/float64(n)

	// 方差与协方差
	var va, vb, cov float64
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			ga := grayVal(a, x, y) - ua
			gb := grayVal(b, x, y) - ub
			va += ga * ga
			vb += gb * gb
			cov += ga * gb
		}
	}
	va /= float64(n)
	vb /= float64(n)
	cov /= float64(n)

	return (2*ua*ub + c1) * (2*cov + c2) / ((ua*ua + ub*ub + c1) * (va + vb + c2))
}

// ChiSquare 卡方检验：评估 LSB 平面是否隐藏了数据。
// 返回 [0,1] 的 p 值（<0.05 表示"极可能存在隐写"）。
// 计算基于单通道（R）的 LSB 奇偶对统计。
func ChiSquare(img *image.NRGBA) float64 {
	if img == nil {
		return 1
	}
	b := img.Bounds()
	// 统计灰度奇/偶 LSB 的直方图
	hist := make([]int, 256)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			// BT.601 灰度（G 权重 150 为偶数，R/B 权重 77/29 为奇数，
			// 奇偶性由 R⊕B 决定，三通道 LSB 独立均匀时奇偶无偏差）。
			v := int(grayVal(img, x, y))
			hist[v]++
		}
	}
	var chi2 float64
	freedom := 0
	for v := 0; v < 256; v += 2 {
		n0, n1 := hist[v], hist[v+1]
		exp := float64(n0+n1) / 2
		if exp > 0 {
			chi2 += (float64(n0)-exp)*(float64(n0)-exp)/exp + (float64(n1)-exp)*(float64(n1)-exp)/exp
			freedom++
		}
	}
	if freedom == 0 {
		return 1
	}
	// 上不完全伽马函数 P(a, x/2)，用数值积分近似
	return igamc(float64(freedom)/2, chi2/2)
}

// RSAnalysis RS 分析（Fridrich）：返回估计的嵌入率 [0,1]。
// 简化实现：比较常规分组与翻转分组的平滑度函数差异。
func RSAnalysis(img *image.NRGBA) float64 {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	n := w * h
	if n == 0 {
		return 0
	}
	// 掩码 [0,1,1,0] 用于分组函数 f
	// 常规分组 vs 翻转分组的平滑度比例（Fridrich RS 简化）
	var regC, regM, invC, invM float64
	// 取前 min(n, 200000) 像素分组（性能控制）
	limit := n
	if limit > 200000 {
		limit = 200000
	}
	for i := 0; i+3 < limit; i += 4 {
		x0, y0 := i%w, i/w
		x1, y1 := (i+1)%w, (i+1)/w
		x2, y2 := (i+2)%w, (i+2)/w
		x3, y3 := (i+3)%w, (i+3)/w
		vals := [4]int{
			int(grayVal(img, x0+b.Min.X, y0+b.Min.Y)),
			int(grayVal(img, x1+b.Min.X, y1+b.Min.Y)),
			int(grayVal(img, x2+b.Min.X, y2+b.Min.Y)),
			int(grayVal(img, x3+b.Min.X, y3+b.Min.Y)),
		}
		f := smoothness(vals[:])
		// 简化：奇偶 LSB 模式近似分组属性
		if vals[0]&1 == vals[1]&1 {
			regC++
			if f > 0 {
				regM++
			}
		} else {
			invC++
			if f > 0 {
				invM++
			}
		}
	}
	if regC == 0 || invC == 0 {
		return 0
	}
	rM := regM / regC
	sM := invM / invC
	if rM <= sM {
		return 0
	}
	// 线性插值近似嵌入率
	p := (rM - sM) / 0.5
	if p > 1 {
		p = 1
	}
	if p < 0 {
		p = 0
	}
	return p
}

// smoothness 分组平滑度函数 f = Σ|x_{i+1}-x_i|。
func smoothness(vals []int) float64 {
	f := float64(0)
	for i := 0; i+1 < len(vals); i++ {
		f += math.Abs(float64(vals[i+1] - vals[i]))
	}
	return f
}

// Analysis 汇总分析结果。
type Analysis struct {
	Width, Height int
	Capacity      int     // 当前深度下容量（位）
	CapacityBytes int     // 容量（字节）
	PSNR          float64 // 相对原图（嵌入后调用时有效）
	SSIM          float64
	ChiSquare     float64 // 卡方 p 值
	EmbedRate     float64 // RS 分析估计嵌入率 [0,1]
}

// Analyze 对单张图做隐写风险评估（嵌入前调用，无原图对比）。
func Analyze(img *image.NRGBA, algo Algorithm, opt Options) *Analysis {
	res := &Analysis{}
	if img == nil {
		return res
	}
	b := img.Bounds()
	res.Width, res.Height = b.Dx(), b.Dy()
	if algo != nil {
		res.Capacity = algo.Capacity(img, opt)
		res.CapacityBytes = res.Capacity / 8
	}
	res.ChiSquare = ChiSquare(img)
	res.EmbedRate = RSAnalysis(img)
	return res
}

// Compare 对比原图与隐写图，给出质量指标。
func Compare(orig, stego *image.NRGBA, algo Algorithm, opt Options) *Analysis {
	res := Analyze(stego, algo, opt)
	if orig != nil && stego != nil {
		res.PSNR = PSNR(orig, stego)
		res.SSIM = SSIM(orig, stego)
	}
	return res
}

// grayVal 像素灰度值（BT.601）。
func grayVal(img *image.NRGBA, x, y int) float64 {
	c := img.NRGBAAt(x, y)
	return (77*float64(c.R) + 150*float64(c.G) + 29*float64(c.B)) / 256
}

func sq(v float64) float64 { return v * v }

// igamc 上不完全伽马函数 P(a, x)（数值级数近似）。
// 用于卡方 CDF：Q(a, x) = 1 - P(a, x)。
func igamc(a, x float64) float64 {
	if x <= 0 {
		return 1
	}
	if x < a+1 {
		// 用级数展开求下不完全伽马，P = 1 - Q
		return 1 - igamP(a, x)
	}
	// 用连分式求 Q
	const maxIter = 200
	const eps = 3e-14
	b := x + 1 - a
	c := 1 / 1e-30
	d := 1 / b
	h := d
	for i := 1; i <= maxIter; i++ {
		an := float64(-i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		c = b + an/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lgamma(a)) * h
}

// igamP 下不完全伽马函数 P(a, x)（级数展开）。
func igamP(a, x float64) float64 {
	const maxIter = 200
	const eps = 3e-14
	sum := 1 / a
	term := sum
	ap := a
	for i := 1; i <= maxIter; i++ {
		ap++
		term *= x / ap
		sum += term
		if math.Abs(term) < math.Abs(sum)*eps {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lgamma(a))
}

// lgamma 自然对数伽马（斯特林近似，a>0 足够精确）。
func lgamma(a float64) float64 {
	// 对 a<8 用递推
	v := a
	var sum float64
	for v < 8 {
		sum -= math.Log(v)
		v++
	}
	// Stirling
	x := v
	const c0 = 0.08333333333333333
	const c1 = -0.002777777777777778
	const c2 = 0.0007936507936507937
	const c3 = -0.0005952380952380953
	const c4 = 0.0008417508417508418
	const c5 = -0.0019175269175269176
	const c6 = 0.00641025641025641
	const c7 = -0.02955065359477124
	lg := (x-0.5)*math.Log(x) - x + 0.5*math.Log(2*math.Pi)
	inv := 1 / x
	inv2 := inv * inv
	ser := c0 + inv2*(c1+inv2*(c2+inv2*(c3+inv2*(c4+inv2*(c5+inv2*(c6+inv2*c7))))))
	return sum + lg + inv*ser
}
