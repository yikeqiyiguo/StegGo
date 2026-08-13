package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"steggo/pkg/steg"
)

func newAuditCmd() *cobra.Command {
	var (
		input   string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "audit -i <载体图片>",
		Short: "隐写自检审计：卡方检验 / RS 分析 / SPA 分析",
		Long: `对载体图片执行三重隐写检测：
  卡方检验 (Chi-Square) — 检测 LSB 平面是否被均匀化
  RS 分析  — 正则/奇异组比例不对称性
  SPA 分析 — 相邻像素对差异奇偶分布

判定为 SUSPICIOUS 仅表示 LSB 平面存在人工均匀化痕迹，并非绝对结论。`,
		Example: `  steggo audit -i cover.png
  steggo audit -i cover.png --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("必须指定 -i 载体图片")
			}
			res, err := steg.AuditImage(input)
			if err != nil {
				return err
			}
			if jsonOut {
				b, _ := json.MarshalIndent(res, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			fmt.Printf("审计目标: %s\n", input)
			fmt.Printf("判定结果: %s\n", colorVerdict(res.Verdict))
			if res.ChiSquare != nil {
				fmt.Printf("卡方检验: P值=%.4f %s\n", res.ChiSquare.PValue, yesNo(res.ChiSquare.Suspected, "异常均匀"))
			}
			if res.RS != nil {
				fmt.Printf("RS 分析 : R_M=%.4f S_M=%.4f 嵌入率≈%.1f%% %s\n",
					res.RS.RM, res.RS.SM, res.RS.EstimatedRate*100, yesNo(res.RS.Suspected, "异常"))
			}
			if res.SPA != nil {
				fmt.Printf("SPA 分析: 奇偶偏斜=%.4f %s\n", res.SPA.Skew, yesNo(res.SPA.Suspected, "异常"))
			}
			for _, d := range res.Details {
				fmt.Printf("  - %s\n", d)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "载体图片路径")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "输出 JSON")
	return cmd
}

func colorVerdict(v string) string {
	if v == "CLEAN" {
		return "[CLEAN] 未发现明显隐写痕迹"
	}
	return "[SUSPICIOUS] 疑似存在隐写痕迹"
}

func yesNo(cond bool, txt string) string {
	if cond {
		return "(异常)"
	}
	return "(正常)"
}
