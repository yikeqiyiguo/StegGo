package steg

import (
	"errors"
	"image"
	"math"

	"steggo/pkg/carrier"
)

// PSNR 计算原始图与隐写图的峰值信噪比 (dB)。
// 典型值：>35dB 人眼几乎无法分辨差异；>30dB 差异可接受。
func PSNR(orig, steg *image.NRGBA) (float64, error) {
	b1, b2 := orig.Bounds(), steg.Bounds()
	if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() {
		return 0, errors.New("图像尺寸不一致")
	}
	var mse float64
	count := 0
	for y := b1.Min.Y; y < b1.Max.Y; y++ {
		for x := b1.Min.X; x < b1.Max.X; x++ {
			a := orig.NRGBAAt(x, y)
			c := steg.NRGBAAt(x, y)
			dr := float64(int(a.R) - int(c.R))
			dg := float64(int(a.G) - int(c.G))
			db := float64(int(a.B) - int(c.B))
			mse += dr*dr + dg*dg + db*db
			count += 3
		}
	}
	if count == 0 {
		return 0, errors.New("图像为空")
	}
	mse /= float64(count)
	if mse == 0 {
		return math.Inf(1), nil
	}
	return 10 * math.Log10(255*255/mse), nil
}

// SSIM 计算结构相似度（0-1，越接近 1 越相似）。
// 实现为全局统计近似（均值/方差/协方差），带亮度与对比度稳定常数。
func SSIM(orig, steg *image.NRGBA) float64 {
	b1, b2 := orig.Bounds(), steg.Bounds()
	if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() {
		return 0
	}
	const C1 = (0.01 * 255) * (0.01 * 255)
	const C2 = (0.03 * 255) * (0.03 * 255)

	var sumX, sumY, sumXY, sumXX, sumYY float64
	var n float64
	for y := b1.Min.Y; y < b1.Max.Y; y++ {
		for x := b1.Min.X; x < b1.Max.X; x++ {
			a := orig.NRGBAAt(x, y)
			c := steg.NRGBAAt(x, y)
			gx := 0.299*float64(a.R) + 0.587*float64(a.G) + 0.114*float64(a.B)
			gy := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
			sumX += gx
			sumY += gy
			sumXY += gx * gy
			sumXX += gx * gx
			sumYY += gy * gy
			n++
		}
	}
	if n == 0 {
		return 0
	}
	meanX := sumX / n
	meanY := sumY / n
	varX := sumXX/n - meanX*meanX
	varY := sumYY/n - meanY*meanY
	covar := sumXY/n - meanX*meanY

	numer := (2*meanX*meanY + C1) * (2*covar + C2)
	denom := (meanX*meanX + meanY*meanY + C1) * (varX + varY + C2)
	if denom == 0 {
		return 0
	}
	return numer / denom
}

// QualityReport 隐写质量报告。
type QualityReport struct {
	PSNR  float64 `json:"psnr_db"`
	SSIM  float64 `json:"ssim"`
	Notes []string `json:"notes"`
}

// EvaluateQuality 对比原始载体与隐写载体的质量。
func EvaluateQuality(origPath, stegPath string) (*QualityReport, error) {
	orig, err := carrier.LoadImage(origPath)
	if err != nil {
		return nil, err
	}
	steg, err := carrier.LoadImage(stegPath)
	if err != nil {
		return nil, err
	}
	psnr, err := PSNR(orig, steg)
	if err != nil {
		return nil, err
	}
	ssim := SSIM(orig, steg)
	r := &QualityReport{PSNR: psnr, SSIM: ssim}
	switch {
	case psnr >= 35:
		r.Notes = append(r.Notes, "PSNR ≥ 35dB：肉眼几乎无法察觉差异")
	case psnr >= 30:
		r.Notes = append(r.Notes, "PSNR ≥ 30dB：差异可接受")
	default:
		r.Notes = append(r.Notes, "PSNR < 30dB：可能存在可见差异")
	}
	if ssim >= 0.98 {
		r.Notes = append(r.Notes, "SSIM ≥ 0.98：结构高度保真")
	} else if ssim >= 0.9 {
		r.Notes = append(r.Notes, "SSIM ≥ 0.9：结构保真良好")
	}
	return r, nil
}
