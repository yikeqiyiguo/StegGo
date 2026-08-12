package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version 版本号，构建时可通过 -ldflags 覆盖。
var Version = "1.0.0"

func main() {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "steggo",
		Short: "StegGo - 抗检测隐写工具 (V1.0 生态版)",
		Long: `StegGo V1.0 生态版 - 抗检测隐写工具

核心链路：ZIP压缩 → PBKDF2派生 → AES-256-GCM加密 → SHA256绑定 → 抗检测LSB/载体容器
自研壁垒：伪随机坐标 LSB + RGB 三通道轮询 + 高斯噪声填充（对抗卡方/RS/SPA 检测）

命令速览：
  hide     嵌入秘密到载体（自动识别载体类型）
  extract  从载体提取秘密
  audit    隐写自检审计（卡方/RS/SPA）
  capacity 载体容量预检测
  quality  隐写质量评估（PSNR/SSIM）
  batch    批量嵌入/提取
  shamir   Shamir 门限分片
  zerowidth 零宽字符隐写
  verify   载体完整性校验
  info     环境与支持信息

仅支持无损载体：PNG/BMP/TIFF/WAV/PDF/TXT/MD/视频；JPG 等有损格式一律拦截。`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolP("quiet", "q", false, "静默模式，减少输出")
	root.AddCommand(
		newHideCmd(),
		newExtractCmd(),
		newAuditCmd(),
		newCapacityCmd(),
		newQualityCmd(),
		newBatchCmd(),
		newShamirCmd(),
		newZeroWidthCmd(),
		newVerifyCmd(),
		newInfoCmd(),
		newVersionCmd(),
	)
	return root
}

// promptPassword 交互式读取密码（隐藏回显）。
func promptPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, err
		}
		if len(pw) == 0 {
			return nil, fmt.Errorf("密码不能为空")
		}
		return pw, nil
	}
	// 非终端（管道）环境：读一行
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return nil, fmt.Errorf("无法读取密码")
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, fmt.Errorf("密码不能为空")
	}
	return []byte(line), nil
}

// resolvePassword 优先使用 flag 传入的密码，否则交互输入。
func resolvePassword(flagPw string, prompt string) ([]byte, error) {
	if flagPw != "" {
		return []byte(flagPw), nil
	}
	return promptPassword(prompt)
}
