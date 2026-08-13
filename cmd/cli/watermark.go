package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"steggo/internal/service"
)

func newWatermarkCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "watermark",
		Short: "数字水印（公开可提取，无需密码）",
		Long: `数字水印用于版权归属声明：以固定种子嵌入 LSB，任何人均可提取，
无需密码。嵌入不影响图像可读性，标记内容不可见。`,
	}
	embed := &cobra.Command{
		Use:   "embed -c <载体> -m <水印> [-o <输出>]",
		Short: "向图像嵌入水印",
		Example: `  steggo watermark embed -c photo.png -m "© 2026 StegGo" -o marked.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			carrier, _ := cmd.Flags().GetString("carrier")
			mark, _ := cmd.Flags().GetString("mark")
			out, _ := cmd.Flags().GetString("output")
			if carrier == "" || mark == "" {
				return fmt.Errorf("必须指定 -c 载体与 -m 水印")
			}
			if out == "" {
				out = carrier + ".wm.png"
			}
			svc := service.New()
			res, err := svc.EmbedWatermark(carrier, out, mark)
			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 水印已嵌入: %s -> %s\n", res.Name, out)
			}
			return nil
		},
	}
	embed.Flags().StringP("carrier", "c", "", "载体图片")
	embed.Flags().StringP("mark", "m", "", "水印内容")
	embed.Flags().StringP("output", "o", "", "输出图片（默认 <载体>.wm.png）")

	extract := &cobra.Command{
		Use:   "extract -c <图片>",
		Short: "从图像提取水印",
		Example: `  steggo watermark extract -c marked.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			carrier, _ := cmd.Flags().GetString("carrier")
			if carrier == "" {
				return fmt.Errorf("必须指定 -c 图片")
			}
			svc := service.New()
			mark, err := svc.ExtractWatermark(carrier)
			if err != nil {
				return err
			}
			fmt.Printf("[OK] 水印: %s\n", mark)
			return nil
		},
	}
	extract.Flags().StringP("carrier", "c", "", "图片路径")

	root.AddCommand(embed, extract)
	return root
}
