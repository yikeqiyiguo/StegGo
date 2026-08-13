package sdk

import (
	"fmt"

	"steggo/internal/algorithm"
	"steggo/internal/carrier"
)

// CapacityInfo 指定算法与位深下的图像容量。
type CapacityInfo struct {
	Algorithm string
	BitDepth  int
	MaxBytes  int64
	Usable    int64
}

// CheckImageCapacity 检查图像在指定算法与位深下的可容纳字节数。
func CheckImageCapacity(path, algo string, bits int) (*CapacityInfo, error) {
	if bits < 1 || bits > 4 {
		return nil, fmt.Errorf("嵌入位数必须是 1-4")
	}
	if algo == "" {
		algo = "lsb"
	}
	alg := algorithm.Get(algo)
	if alg == nil {
		return nil, fmt.Errorf("未知算法: %s", algo)
	}
	img, err := carrier.LoadImage(path)
	if err != nil {
		return nil, err
	}
	var opt algorithm.Options
	opt.BitDepth = bits
	if err := opt.Normalize(); err != nil {
		return nil, err
	}
	capBytes := int64(alg.Capacity(img, opt) / 8)
	return &CapacityInfo{Algorithm: algo, BitDepth: bits, MaxBytes: capBytes, Usable: capBytes}, nil
}

// CapacityMatrix 计算图像在指定算法下 1-4 位深的容量矩阵。
func CapacityMatrix(path, algo string) ([]*CapacityInfo, error) {
	var mat []*CapacityInfo
	for bits := 1; bits <= 4; bits++ {
		r, err := CheckImageCapacity(path, algo, bits)
		if err != nil {
			return nil, err
		}
		mat = append(mat, r)
	}
	return mat, nil
}

// QualityReport 隐写前后图像质量评估结果。
type QualityReport struct {
	PSNR  float64
	SSIM  float64
	Notes []string
}

// EvaluateQuality 评估隐写后图像相对原图的 PSNR 与 SSIM。
func EvaluateQuality(orig, steg string) (*QualityReport, error) {
	a, err := carrier.LoadImage(orig)
	if err != nil {
		return nil, err
	}
	b, err := carrier.LoadImage(steg)
	if err != nil {
		return nil, err
	}
	psnr := algorithm.PSNR(a, b)
	ssim := algorithm.SSIM(a, b)
	rep := &QualityReport{PSNR: psnr, SSIM: ssim}
	switch {
	case psnr >= 40:
		rep.Notes = append(rep.Notes, "PSNR ≥ 40dB：差异肉眼难以察觉")
	case psnr >= 30:
		rep.Notes = append(rep.Notes, "PSNR 30-40dB：轻微可见差异")
	default:
		rep.Notes = append(rep.Notes, "PSNR < 30dB：差异明显，需降低嵌入强度")
	}
	if ssim > 0.95 {
		rep.Notes = append(rep.Notes, "SSIM > 0.95：结构保持良好")
	} else {
		rep.Notes = append(rep.Notes, "SSIM ≤ 0.95：结构存在可见失真")
	}
	return rep, nil
}

// AuditReport 自检审计（卡方 + RS）结果。
type AuditReport struct {
	Verdict   string
	ChiSquare float64
	EmbedRate float64
	Width     int
	Height    int
	Capacity  int64
	Details   []string
}

// AnalyzeImage 对单张图像做隐写风险评估（嵌入前调用，无原图对比）。
func AnalyzeImage(path string) (*AuditReport, error) {
	img, err := carrier.LoadImage(path)
	if err != nil {
		return nil, err
	}
	alg := algorithm.Get("lsb")
	if alg == nil {
		return nil, fmt.Errorf("LSB 算法未注册")
	}
	var opt algorithm.Options
	if err := opt.Normalize(); err != nil {
		return nil, err
	}
	an := algorithm.Analyze(img, alg, opt)
	res := &AuditReport{
		ChiSquare: an.ChiSquare,
		EmbedRate: an.EmbedRate,
		Width:     an.Width,
		Height:    an.Height,
		Capacity:  int64(an.CapacityBytes),
	}
	switch {
	case an.ChiSquare < 0.05 && an.EmbedRate > 0.1:
		res.Verdict = "高风险：疑似存在隐写（卡方 + RS 双重异常）"
	case an.ChiSquare < 0.05:
		res.Verdict = "可疑：LSB 奇偶分布异常（卡方偏低）"
	case an.EmbedRate > 0.1:
		res.Verdict = "可疑：RS 分析嵌入率偏高"
	default:
		res.Verdict = "正常：未检测到明显隐写痕迹"
	}
	res.Details = append(res.Details,
		fmt.Sprintf("尺寸 %dx%d", an.Width, an.Height),
		fmt.Sprintf("LSB 容量 %d 字节", an.CapacityBytes),
	)
	return res, nil
}
