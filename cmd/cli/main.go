package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// version 版本号，构建时可通过 -ldflags "-X main.version=v2.2.0" 注入。
var version = "2.2.0"

// Version 兼容引用（info/version 命令显示用）。
var Version = version

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
		Short: "StegGo - 抗检测隐写工具 (V2.0)",
		Long: `StegGo V2.0 - 抗检测隐写工具

核心链路：ZIP压缩 -> 三因子密钥派生(PBKDF2) -> AES-256-GCM加密 -> SHA256绑定
           -> 七算法隐写(LSB/DCT/DWT/HUGO/WOW/UNIWARD/锚定) -> 载体容器/套娃/Polyglot
架构：五层(common/crypto/algorithm/carrier/service) + 三层交互(CLI/TUI/GUI) + 离线铁则
自研壁垒：确定性伪随机游走 + 成本加权嵌入 + 高斯噪声填充（对抗卡方/RS/SPA 检测）

命令速览：
  hide      嵌入秘密到载体（七算法 + 三因子 + 可否认）
  extract   从载体提取秘密（自动扫描算法 + V1 兼容）
  watermark 数字水印（公开可提取）
  nested    套娃递归嵌套隐写
  audit     隐写自检审计（卡方/RS/SPA）
  capacity  载体容量预检测
  quality   隐写质量评估（PSNR/SSIM）
  batch     批量嵌入/提取
  shamir    Shamir 门限分片
  zerowidth 零宽字符隐写
  verify    载体完整性校验
  kyber     后量子加密 ML-KEM-768（密钥对生成）
  plugin    插件加载框架（查看已注册插件）
  info      环境与支持信息

仅支持无损载体：PNG/BMP/TIFF/WAV/FLAC/PDF/TXT/MD/视频；JPG 等有损格式一律拦截。`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolP("quiet", "q", false, "静默模式，减少输出")
	registerBuiltinPlugins()
	root.AddCommand(
		newHideCmd(),
		newExtractCmd(),
		newWatermarkCmd(),
		newNestedCmd(),
		newAuditCmd(),
		newCapacityCmd(),
		newQualityCmd(),
		newBatchCmd(),
		newShamirCmd(),
		newZeroWidthCmd(),
		newVerifyCmd(),
		newSGCmd(),
		newLedgerCmd(),
		newTaskCmd(),
		newScheduleCmd(),
		newKyberCmd(),
		newPluginCmd(),
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

// resolvePasswordEx 支持 --password 与 --password-file（文件取首行，自动去空白）。
func resolvePasswordEx(flagPw, flagFile, prompt string) ([]byte, error) {
	if flagPw != "" {
		return []byte(flagPw), nil
	}
	if flagFile != "" {
		data, err := os.ReadFile(flagFile)
		if err != nil {
			return nil, fmt.Errorf("读取密码文件: %w", err)
		}
		line := strings.TrimSpace(string(data))
		if line == "" {
			return nil, fmt.Errorf("密码文件为空")
		}
		return []byte(line), nil
	}
	return promptPassword(prompt)
}
