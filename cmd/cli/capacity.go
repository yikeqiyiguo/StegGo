package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"steggo/pkg/carrier"
	"steggo/pkg/steg"
)

func newCapacityCmd() *cobra.Command {
	var (
		input   string
		bits    int
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "capacity -i <载体>",
		Short: "载体容量预检测",
		Long: `检测载体可容纳的秘密大小，避免嵌入中途失败。

  图片载体：按像素×通道×位深度计算理论容量
  音频/PDF/视频载体：按文件大小估算可用空间`,
		Example: `  steggo capacity -i cover.png
  steggo capacity -i cover.png -b 3
  steggo capacity -i cover.png --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("必须指定 -i 载体")
			}
			kind, err := carrier.DetectKind(input)
			if err != nil {
				return err
			}
			switch kind {
			case carrier.KindImage:
				mat, err := steg.CapacityMatrix(input)
				if err != nil {
					return err
				}
				if jsonOut {
					b, _ := json.MarshalIndent(mat, "", "  ")
					fmt.Println(string(b))
					return nil
				}
				fmt.Printf("载体: %s (%dx%d)\n", input, mat[0].Width, mat[0].Height)
				fmt.Printf("%-10s %-16s %-16s\n", "位深度", "理论容量", "可用容量(扣头部)")
				for _, r := range mat {
					fmt.Printf("%-10d %-16s %-16s\n", r.BitDepth, humanSize(r.MaxBytes), humanSize(r.Usable))
				}
				if bits > 0 {
					r, err := steg.CheckImageCapacity(input, bits)
					if err != nil {
						return err
					}
					fmt.Printf("\n指定位深度 %d: 可用 %s\n", bits, humanSize(r.Usable))
				}
			default:
				size, err := steg.CheckGenericCapacity(input)
				if err != nil {
					return err
				}
				if jsonOut {
					b, _ := json.Marshal(map[string]interface{}{"kind": kind.String(), "file_size": size})
					fmt.Println(string(b))
					return nil
				}
				fmt.Printf("载体: %s (%s)\n", input, humanSize(size))
				fmt.Printf("[+] 通用载体（%s）容量受文件大小限制，建议秘密不超过文件大小的 30%%\n", kind.String())
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "载体路径")
	cmd.Flags().IntVarP(&bits, "bits", "b", 0, "查看指定位深度的容量")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "输出 JSON")
	return cmd
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

var _ = os.Exit
