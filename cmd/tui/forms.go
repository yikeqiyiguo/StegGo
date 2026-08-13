package main

import (
	"fmt"

	"steggo/internal/service"
)

func hideForm() *formSpec {
	return &formSpec{
		title: "嵌入 (Hide)",
		fields: []fieldSpec{
			{key: "carrier", label: "载体文件 (PNG/BMP/TIFF/WAV/PDF/TXT/MD/视频)"},
			{key: "secret", label: "秘密文件路径"},
			{key: "output", label: "输出文件 (留空自动生成)"},
			{key: "password", label: "加密密码", secret: true},
			{key: "algorithm", label: "算法 lsb/dct/dwt/hugo/wow/uniward (默认 lsb)", optional: true},
			{key: "bits", label: "嵌入位数 1-4 (默认1)", optional: true},
		},
		run: func(v map[string]string) (string, error) {
			out := v["output"]
			if out == "" {
				out = v["carrier"] + ".steg.png"
			}
			bits := 1
			if v["bits"] != "" {
				if _, err := fmt.Sscanf(v["bits"], "%d", &bits); err != nil {
					return "", fmt.Errorf("嵌入位数必须是数字")
				}
			}
			opt := service.Options{
				CarrierPath: v["carrier"],
				SecretPath:  v["secret"],
				OutputPath:  out,
				Password:    []byte(v["password"]),
				Algorithm:   v["algorithm"],
				BitDepth:    bits,
			}
			res, err := service.New().Embed(opt)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("嵌入成功: %s (%d B) -> %s [算法=%s 位深=%d]",
				res.Name, res.Size, out, res.Algorithm, res.BitDepth), nil
		},
	}
}

func extractForm() *formSpec {
	return &formSpec{
		title: "提取 (Extract)",
		fields: []fieldSpec{
			{key: "carrier", label: "隐写载体文件"},
			{key: "output", label: "输出目录 (留空默认 ./extracted)"},
			{key: "password", label: "解密密码", secret: true},
		},
		run: func(v map[string]string) (string, error) {
			out := v["output"]
			if out == "" {
				out = "./extracted"
			}
			res, err := service.New().Extract(service.Options{
				CarrierPath: v["carrier"],
				OutputPath:  out,
				Password:    []byte(v["password"]),
			})
			if err != nil {
				return "", err
			}
			extra := ""
			if res.V1Compat {
				extra = " [V1兼容]"
			} else {
				extra = fmt.Sprintf(" [算法=%s]", res.Algorithm)
			}
			return fmt.Sprintf("提取成功: %s (%d B) -> %s/%s", res.Name, res.Size, out, extra), nil
		},
	}
}

func watermarkForm() *formSpec {
	return &formSpec{
		title: "数字水印 (Watermark)",
		fields: []fieldSpec{
			{key: "carrier", label: "载体图片路径"},
			{key: "output", label: "输出图片 (留空自动生成)"},
			{key: "mark", label: "水印内容 (无需密码)"},
		},
		run: func(v map[string]string) (string, error) {
			out := v["output"]
			if out == "" {
				out = v["carrier"] + ".wm.png"
			}
			res, err := service.New().EmbedWatermark(v["carrier"], out, v["mark"])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("水印已嵌入: %s -> %s", res.Name, out), nil
		},
	}
}

func auditForm() *formSpec {
	return &formSpec{
		title: "自检审计 (Audit)",
		fields: []fieldSpec{
			{key: "input", label: "载体图片路径"},
		},
		run: func(v map[string]string) (string, error) {
			res, err := stegAuditImage(v["input"])
			if err != nil {
				return "", err
			}
			out := "审计目标: " + v["input"] + "\n"
			out += "判定结果: " + res.Verdict + "\n"
			if res.ChiSquare != nil {
				out += fmt.Sprintf("卡方检验: P=%.4f\n", res.ChiSquare.PValue)
			}
			if res.RS != nil {
				out += fmt.Sprintf("RS分析  : 嵌入率≈%.1f%%\n", res.RS.EstimatedRate*100)
			}
			if res.SPA != nil {
				out += fmt.Sprintf("SPA分析 : 偏斜=%.4f\n", res.SPA.Skew)
			}
			for _, d := range res.Details {
				out += "  - " + d + "\n"
			}
			return out, nil
		},
	}
}

func capacityForm() *formSpec {
	return &formSpec{
		title: "容量检测 (Capacity)",
		fields: []fieldSpec{
			{key: "input", label: "载体路径"},
			{key: "bits", label: "嵌入位数 1-4 (可选，仅图片)", optional: true},
		},
		run: func(v map[string]string) (string, error) {
			if v["bits"] != "" {
				var bits int
				if _, err := fmt.Sscanf(v["bits"], "%d", &bits); err != nil {
					return "", fmt.Errorf("嵌入位数必须是数字")
				}
				r, err := stegCheckImageCapacity(v["input"], bits)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("%s: 位深度%d 可用容量 %d B", v["input"], r.BitDepth, r.Usable), nil
			}
			mat, err := stegCapacityMatrix(v["input"])
			if err != nil {
				return "", err
			}
			out := ""
			for _, r := range mat {
				out += fmt.Sprintf("位深度 %d: 理论 %d B, 可用 %d B\n", r.BitDepth, r.MaxBytes, r.Usable)
			}
			return out, nil
		},
	}
}

func qualityForm() *formSpec {
	return &formSpec{
		title: "质量评估 (Quality)",
		fields: []fieldSpec{
			{key: "orig", label: "原始载体图片"},
			{key: "steg", label: "隐写后载体图片"},
		},
		run: func(v map[string]string) (string, error) {
			rep, err := stegEvaluateQuality(v["orig"], v["steg"])
			if err != nil {
				return "", err
			}
			out := fmt.Sprintf("PSNR: %.2f dB\nSSIM: %.6f\n", rep.PSNR, rep.SSIM)
			for _, n := range rep.Notes {
				out += "  - " + n + "\n"
			}
			return out, nil
		},
	}
}
