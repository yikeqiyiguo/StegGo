package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"steggo/pkg/steg"
)

func newQualityCmd() *cobra.Command {
	var (
		orig    string
		stegF   string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "quality --orig <原始载体> --steg <隐写载体>",
		Short: "隐写质量评估：PSNR / SSIM",
		Long: `对比原始载体与隐写载体的质量指标：

  PSNR (峰值信噪比): >35dB 人眼几乎无法察觉差异
  SSIM (结构相似度): 越接近 1 结构保真度越高`,
		Example: `  steggo quality --orig cover.png --steg cover.png.steg.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if orig == "" || stegF == "" {
				return fmt.Errorf("必须指定 --orig 与 --steg")
			}
			rep, err := steg.EvaluateQuality(orig, stegF)
			if err != nil {
				return err
			}
			if jsonOut {
				b, _ := json.MarshalIndent(rep, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			fmt.Printf("原始载体: %s\n", orig)
			fmt.Printf("隐写载体: %s\n", stegF)
			fmt.Printf("PSNR : %.2f dB\n", rep.PSNR)
			fmt.Printf("SSIM : %.6f\n", rep.SSIM)
			for _, n := range rep.Notes {
				fmt.Printf("  - %s\n", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orig, "orig", "", "原始载体图片")
	cmd.Flags().StringVar(&stegF, "steg", "", "隐写后载体图片")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "输出 JSON")
	return cmd
}
