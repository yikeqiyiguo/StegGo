package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"steggo/internal/service"
)

func newHideCmd() *cobra.Command {
	var (
		carrierPath string
		secret      string
		output      string
		pass        string
		bits        int
		mask        int
		dirMode     bool
		name        string
		algorithm   string
		quality     int
		levels      int
		cost        string
		keyfile     string
		fakeFile    string
		fakePass    string
		useMachine  bool
	)
	cmd := &cobra.Command{
		Use:   "hide -c <载体> -s <秘密> [-o <输出>] [-p <密码>]",
		Short: "将秘密文件/目录嵌入载体（V2.0 六算法 + 三因子 + 可否认）",
		Long: `将秘密文件（或 --dir 目录）加密后嵌入载体。

算法（图片载体）：
  --algorithm lsb|dct|dwt|hugo|wow|uniward （默认 lsb）
  --bits 每通道嵌入位数(1-4)；--quality DCT量化步长；--levels DWT级数
  --cost 自适应成本函数 hill|wow|uniward

载体类型自动识别（魔数优先，防伪装）：
  图片 PNG/BMP/TIFF -> 算法嵌入（伪随机游走 + 确定性噪声填充）
  音频 WAV/FLAC、PDF、视频 -> 尾部加密容器
  文本 TXT/MD        -> 零宽字符隐写
有损格式（JPG/MP3/AAC/OGG/M4A/WMA）强制拦截。

安全特性：
  --keyfile 密钥文件（三因子之一）；--machine 绑定本机指纹
  --fake-file <诱饵> --fake-pass <诱饵密码> 可否认胁迫隐写（双密文）`,
		Example: `  steggo hide -c cover.png -s secret.txt -p 密码
  steggo hide -c cover.png -s ./mydir --dir -o out.png
  steggo hide -c cover.png -s secret.txt --algorithm dct --quality 8
  steggo hide -c cover.png -s secret.txt --fake-file fake.txt --fake-pass 假密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if carrierPath == "" || secret == "" {
				return fmt.Errorf("必须指定 -c 载体与 -s 秘密文件")
			}
			svc := service.New()
			opt := service.Options{
				CarrierPath: carrierPath,
				SecretPath:  secret,
				OutputPath:  outputOrDefault(output, carrierPath),
				Password:    password,
				Algorithm:   algorithm,
				BitDepth:    bits,
				ChannelMask: mask,
				Quality:     quality,
				Levels:      levels,
				CostStyle:   cost,
				Name:        name,
				IsDir:       dirMode,
			}
			if keyfile != "" {
				kf, err := os.ReadFile(keyfile)
				if err != nil {
					return fmt.Errorf("读取密钥文件: %w", err)
				}
				defer wipe(kf)
				opt.KeyFile, opt.UseKeyFile = kf, true
			}
			opt.UseMachine = useMachine
			if fakeFile != "" {
				opt.FakeFile = fakeFile
				opt.FakePassword = []byte(fakePass)
			}
			res, err := svc.Embed(opt)
			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 嵌入完成: %s (%d B) -> %s\n", res.Name, res.Size, opt.OutputPath)
				if res.Deniable {
					fmt.Println("[+] 可否认双密文已写入")
				}
				if res.IsDir {
					fmt.Println("[+] 已打包目录")
				}
				fmt.Printf("[+] 算法=%s 位深=%d\n", res.Algorithm, res.BitDepth)
				fmt.Printf("[+] 输出载体请勿再经过任何有损转换（如另存为 JPG）\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&carrierPath, "carrier", "c", "", "载体文件（PNG/BMP/TIFF/WAV/FLAC/PDF/TXT/MD/视频）")
	cmd.Flags().StringVarP(&secret, "secret", "s", "", "秘密文件或目录（配合 --dir）")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出文件（默认 <载体名>.steg.<原扩展名>）")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "加密密码（不传则交互输入）")
	cmd.Flags().IntVarP(&bits, "bits", "b", 1, "图片每通道嵌入位数 (1-4)")
	cmd.Flags().IntVar(&mask, "mask", 0, "通道掩码 bit0=R bit1=G bit2=B (默认全开)")
	cmd.Flags().BoolVar(&dirMode, "dir", false, "将秘密目录整体打包嵌入")
	cmd.Flags().StringVar(&name, "name", "", "保存的文件名（默认取秘密文件名）")
	cmd.Flags().StringVarP(&algorithm, "algorithm", "a", "lsb", "图片算法: lsb|dct|dwt|hugo|wow|uniward")
	cmd.Flags().IntVar(&quality, "quality", 0, "DCT 量化步长 (1-32)")
	cmd.Flags().IntVar(&levels, "levels", 0, "DWT 分解级数 (1-3)")
	cmd.Flags().StringVar(&cost, "cost", "", "自适应成本函数: hill|wow|uniward")
	cmd.Flags().StringVar(&keyfile, "keyfile", "", "密钥文件（三因子）")
	cmd.Flags().BoolVar(&useMachine, "machine", false, "绑定本机指纹（三因子）")
	cmd.Flags().StringVar(&fakeFile, "fake-file", "", "可否认诱饵文件路径")
	cmd.Flags().StringVar(&fakePass, "fake-pass", "", "可否认诱饵密码")
	return cmd
}

func outputOrDefault(output, carrierPath string) string {
	if output != "" {
		return output
	}
	return carrierPath + ".steg.png"
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
