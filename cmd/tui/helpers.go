package main

import (
	"fmt"

	"steggo/internal/algorithm"
	"steggo/internal/carrier"
)

// =============================================================
// 自检审计辅助函数（基于 V2 algorithm 分析层）
// =============================================================

type chiResult struct {
	PValue float64
}

type rsResult struct {
	EstimatedRate float64
}

type spaResult struct {
	Skew float64
}

type auditResult struct {
	Verdict   string
	ChiSquare *chiResult
	RS        *rsResult
	SPA       *spaResult
	Details   []string
}

func stegAuditImage(path string) (*auditResult, error) {
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
	res := &auditResult{
		ChiSquare: &chiResult{PValue: an.ChiSquare},
		RS:        &rsResult{EstimatedRate: an.EmbedRate},
		SPA:       &spaResult{Skew: an.EmbedRate},
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

// =============================================================
// 容量检测辅助函数
// =============================================================

type capacityResult struct {
	BitDepth int
	MaxBytes int64
	Usable   int64
}

func stegCheckImageCapacity(path string, bits int) (*capacityResult, error) {
	if bits < 1 || bits > 4 {
		return nil, fmt.Errorf("嵌入位数必须是 1-4")
	}
	c := carrier.Get(carrier.KindImage)
	if c == nil {
		return nil, fmt.Errorf("图像载体未注册")
	}
	opt := carrier.Options{Algorithm: "lsb", BitDepth: bits, Seed: []byte("capacity-seed")}
	capBytes, err := c.Capacity(path, opt)
	if err != nil {
		return nil, err
	}
	return &capacityResult{BitDepth: bits, MaxBytes: capBytes, Usable: capBytes}, nil
}

func stegCapacityMatrix(path string) ([]*capacityResult, error) {
	var mat []*capacityResult
	for bits := 1; bits <= 4; bits++ {
		r, err := stegCheckImageCapacity(path, bits)
		if err != nil {
			return nil, err
		}
		mat = append(mat, r)
	}
	return mat, nil
}

// =============================================================
// 质量评估辅助函数
// =============================================================

type qualityReport struct {
	PSNR  float64
	SSIM  float64
	Notes []string
}

func stegEvaluateQuality(orig, steg string) (*qualityReport, error) {
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
	rep := &qualityReport{PSNR: psnr, SSIM: ssim}
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
