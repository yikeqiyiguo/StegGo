package steg

import (
	"image"
	"math"

	"steggo/pkg/carrier"
)

// =============================================================
// 隐写自检审计（Anti-Steganalysis）
//
//  1. 卡方检验 (Chi-Square)  — Westfeld & Pfitzmann 经典检测
//  2. RS 分析 (Regular/Singular)
//  3. SPA 分析 (Sample Pair Analysis)
//
// 自然图像的 LSB 平面存在固有偏斜；隐写/噪声填充会使其趋向均匀。
// 审计工具据此给出 CLEAN / SUSPICIOUS 判定（工程近似，非绝对）。
// =============================================================

// AuditResult 审计结果。
type AuditResult struct {
	ChiSquare *ChiSquareResult `json:"chi_square,omitempty"`
	RS        *RSResult        `json:"rs,omitempty"`
	SPA       *SPAResult       `json:"spa,omitempty"`
	Verdict   string           `json:"verdict"` // CLEAN / SUSPICIOUS
	Details   []string         `json:"details"`
}

// ChiSquareResult 卡方检验结果。
type ChiSquareResult struct {
	ChiSq     float64 `json:"chi_sq"`
	PValue    float64 `json:"p_value"` // 上尾概率；越接近 1 越均匀（疑似嵌入）
	Suspected bool    `json:"suspected"`
}

// RSResult RS 分析结果。
type RSResult struct {
	RM            float64 `json:"rm"`
	SM            float64 `json:"sm"`
	RMNeg         float64 `json:"rm_neg"`
	SMNeg         float64 `json:"sm_neg"`
	EstimatedRate float64 `json:"estimated_rate"` // 近似嵌入率
	Suspected     bool    `json:"suspected"`
}

// SPAResult SPA 分析结果。
type SPAResult struct {
	EvenDiffRatio float64 `json:"even_diff_ratio"`
	OddDiffRatio  float64 `json:"odd_diff_ratio"`
	Skew          float64 `json:"skew"` // 偶数差占比偏斜；嵌入后下降/转负
	Suspected     bool    `json:"suspected"`
}

// AuditImage 对载体图片执行全链路自检审计。
func AuditImage(path string) (*AuditResult, error) {
	img, err := carrier.LoadImage(path)
	if err != nil {
		return nil, err
	}
	return AuditNRGBA(img), nil
}

// AuditNRGBA 对图像执行审计。
func AuditNRGBA(img *image.NRGBA) *AuditResult {
	res := &AuditResult{}
	res.ChiSquare = chiSquareAudit(img)
	res.RS = rsAudit(img)
	res.SPA = spaAudit(img)

	if res.ChiSquare.Suspected {
		res.Details = append(res.Details, "卡方检验：LSB 平面统计异常均匀，疑似存在隐写")
	}
	if res.RS.Suspected {
		res.Details = append(res.Details, "RS 分析：正则/奇异组不对称，疑似存在隐写")
	}
	if res.SPA.Suspected {
		res.Details = append(res.Details, "SPA 分析：像素对差异奇偶分布异常，疑似存在隐写")
	}
	if len(res.Details) == 0 {
		res.Verdict = "CLEAN"
		res.Details = append(res.Details, "三项检测均未发现明显异常，通过自检")
	} else {
		res.Verdict = "SUSPICIOUS"
	}
	return res
}

// =============================================================
// 1. 卡方检验
// =============================================================

func chiSquareAudit(img *image.NRGBA) *ChiSquareResult {
	b := img.Bounds()
	var pSum float64
	var n int
	for _, channel := range []int{0, 1, 2} {
		hist := make([]int, 256)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := img.NRGBAAt(x, y)
				switch channel {
				case 0:
					hist[c.R]++
				case 1:
					hist[c.G]++
				case 2:
					hist[c.B]++
				}
			}
		}
		var chiSq float64
		df := 0
		for k := 0; k < 128; k++ {
			even := hist[2*k]
			odd := hist[2*k+1]
			expected := float64(even+odd) / 2
			if expected > 5 {
				chiSq += (float64(even) - expected) * (float64(even) - expected) / expected
				df++
			}
		}
		if df >= 16 { // 有效对太少（纯色/低熵图）不参与判定
			pSum += 1 - gammaP(float64(df)/2, chiSq/2)
			n++
		}
	}
	if n == 0 {
		return &ChiSquareResult{}
	}
	p := pSum / float64(n)
	return &ChiSquareResult{
		ChiSq:     0,
		PValue:    p,
		Suspected: p > 0.05,
	}
}

// =============================================================
// 2. RS 分析
// =============================================================

const rsMaxSamples = 400_000

func rsAudit(img *image.NRGBA) *RSResult {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := make([]uint8, w*h)
	idx := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			gray[idx] = uint8(0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B))
			idx++
		}
	}

	mask := []int{0, 1, 1, 0}
	var rm, sm, rmn, smn float64
	samples := 0
	flat := 0 // 平坦组（f=0）计数
	for i := 0; i+3 < len(gray) && samples < rsMaxSamples; i += 4 {
		g := []int{int(gray[i]), int(gray[i+1]), int(gray[i+2]), int(gray[i+3])}
		f0 := rsF(g)
		if f0 == 0 {
			flat++
		}
		fm := rsF(rsFlip(g, mask, true))
		fn := rsF(rsFlip(g, mask, false))
		switch {
		case fm > f0:
			rm++
		case fm < f0:
			sm++
		}
		switch {
		case fn > f0:
			rmn++
		case fn < f0:
			smn++
		}
		samples++
	}
	if samples == 0 {
		return &RSResult{}
	}
	// 低纹理图像（平坦组占比过高）无法可靠判定，直接 CLEAN
	if float64(flat)/float64(samples) > 0.9 {
		return &RSResult{}
	}
	RM := rm / float64(samples)
	SM := sm / float64(samples)
	RMN := rmn / float64(samples)
	SMN := smn / float64(samples)

	// 嵌入率近似估计：基于正则/奇异组的正反向掩码对称性差异
	// 干净图像 R_M≈R_-M、S_M≈S_-M；嵌入后 R_M 上升、S_M 下降，
	// 对称性差 metric = (R_M-S_M)-(R_-M-S_-M) 单调增大。
	metric := (RM - SM) - (RMN - SMN)
	est := metric
	if est < 0 {
		est = 0
	}
	if est > 1 {
		est = 1
	}
	return &RSResult{
		RM:            RM,
		SM:            SM,
		RMNeg:         RMN,
		SMNeg:         SMN,
		EstimatedRate: est,
		Suspected:     metric > 0.15,
	}
}

func rsF(g []int) int {
	return abs(g[1]-g[0]) + abs(g[2]-g[1]) + abs(g[3]-g[2])
}

// rsFlip 按掩码翻转像素。forward=true 用 F1（翻转LSB），false 用 F-1。
func rsFlip(g []int, mask []int, forward bool) []int {
	out := make([]int, len(g))
	for i, v := range g {
		out[i] = v
		if mask[i] == 1 {
			if forward {
				out[i] = v ^ 1
			} else {
				if v%2 == 0 {
					if v == 0 {
						out[i] = 0
					} else {
						out[i] = v - 1
					}
				} else {
					out[i] = v + 1
				}
			}
		}
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// =============================================================
// 3. SPA 分析（工程近似）
// =============================================================

const spaMaxSamples = 400_000

func spaAudit(img *image.NRGBA) *SPAResult {
	b := img.Bounds()
	var even, odd float64
	samples := 0
	for y := b.Min.Y; y < b.Max.Y && samples < spaMaxSamples; y++ {
		for x := b.Min.X; x+1 < b.Max.X && samples < spaMaxSamples; x++ {
			a := img.NRGBAAt(x, y)
			c := img.NRGBAAt(x+1, y)
			for _, pair := range [][2]uint8{{a.R, c.R}, {a.G, c.G}, {a.B, c.B}} {
				lo, hi := pair[0], pair[1]
				if lo == hi {
					continue
				}
				if lo > hi {
					lo, hi = hi, lo
				}
				d := int(hi - lo)
				if d%2 == 0 {
					even++
				} else {
					odd++
				}
				samples++
			}
		}
	}
	if samples < 1000 {
		// 样本太少（纯色/极低纹理图像）无法给出可靠结论，判定 CLEAN
		return &SPAResult{EvenDiffRatio: 0, OddDiffRatio: 0, Skew: 0, Suspected: false}
	}
	evenRatio := even / float64(samples)
	oddRatio := odd / float64(samples)
	skew := (even - odd) / (even + odd)
	// 保守判定：仅当奇数差极端占优（skew<-0.3，超出绝大多数自然图像范围）才提示
	suspected := skew < -0.3
	return &SPAResult{
		EvenDiffRatio: evenRatio,
		OddDiffRatio:  oddRatio,
		Skew:          skew,
		Suspected:     suspected,
	}
}

// =============================================================
// 数学工具：正则化不完全伽马函数（用于卡方分布上尾概率）
// =============================================================

func gammaP(a, x float64) float64 {
	if x < 0 || a <= 0 {
		return 0
	}
	if x == 0 {
		return 0
	}
	if x < a+1 {
		return gammaPSeries(a, x)
	}
	return 1 - gammaQCF(a, x)
}

func gammaPSeries(a, x float64) float64 {
	ap := a
	sum := 1.0 / a
	del := sum
	for i := 0; i < 200; i++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*1e-14 {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-logGamma(a))
}

func gammaQCF(a, x float64) float64 {
	const tiny = 1e-300
	b := x + 1 - a
	c := 1.0 / tiny
	d := 1.0 / b
	h := d
	for i := 1; i <= 200; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-14 {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-logGamma(a)) * h
}

// logGamma 使用 Lanczos 近似计算 ln Γ(x)。
func logGamma(x float64) float64 {
	cof := []float64{
		0.99999999999980993, 676.5203681218851, -1259.1392167224028,
		771.32342877765313, -176.61502916214059, 12.507343278686905,
		-0.13857109526572012, 9.9843695780195716e-6, 1.5056327351493116e-7,
	}
	if x < 0.5 {
		return math.Log(math.Pi) - math.Log(math.Sin(math.Pi*x)) - logGamma(1-x)
	}
	x--
	a := cof[0]
	t := x + 7 + 0.5
	for i := 1; i < len(cof); i++ {
		a += cof[i] / (x + float64(i))
	}
	return math.Log(math.Sqrt(2*math.Pi)) + (x+0.5)*math.Log(t) - t + math.Log(a)
}
