package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "显示环境与支持信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("=== StegGo 环境信息 ===")
			fmt.Printf("版本     : %s\n", Version)
			fmt.Printf("平台     : %s/%s\n", runtime.GOOS, runtime.GOARCH)
			fmt.Printf("Go 版本  : %s\n", runtime.Version())
			fmt.Println()
			fmt.Println("支持的载体:")
			fmt.Println("  [图片]  PNG / BMP / TIFF  (LSB/DCT/DWT/HUGO/WOW/UNIWARD, 位深度 1-4)")
			fmt.Println("  [音频]  WAV / FLAC (无损尾部加密容器)")
			fmt.Println("  [文档]  PDF (EOF 标记前冗余数据流, 不破坏渲染结构)")
			fmt.Println("  [文本]  TXT / MD (零宽字符隐写)")
			fmt.Println("  [视频]  MP4/AVI/MKV/MOV/WEBM/FLV (帧分片+XOR冗余)")
			fmt.Println()
			fmt.Println("拦截格式: JPG/JPEG、MP3/AAC/OGG/M4A/WMA 等有损格式")
			fmt.Println()
			fmt.Println("加密体系: ZIP压缩 → 三因子PBKDF2(21万次) → AES-256-GCM → SHA256绑定")
			fmt.Println("隐写算法: LSB / DCT-QIM / DWT-QIM / HUGO / WOW / UNIWARD (成本加权自适应)")
			fmt.Println("自检审计: 卡方检验 / RS 分析 / SPA 分析")
			return nil
		},
	}
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本号",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("StegGo v%s\n", Version)
		},
	}
}
