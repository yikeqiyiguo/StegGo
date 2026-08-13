package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"steggo/pkg/steg"
)

func newZeroWidthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zerowidth",
		Short: "零宽字符隐写（文本载体 TXT/MD）",
		Long: `将秘密以不可见零宽字符 (U+200B/U+200C) 嵌入文本末尾，
肉眼完全不可见，文本渲染零感知。数据仍走完整加密链路。`,
	}
	cmd.AddCommand(newZWEncodeCmd(), newZWDecodeCmd())
	return cmd
}

func newZWEncodeCmd() *cobra.Command {
	var (
		carrier string
		secret  string
		output  string
		pass    string
	)
	cmd := &cobra.Command{
		Use:   "encode -c <文本载体> -s <秘密> -o <输出> [-p <密码>]",
		Short: "将秘密以零宽字符嵌入文本",
		Example: `  steggo zerowidth encode -c note.md -s secret.txt -o note.steg.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if carrier == "" || secret == "" {
				return fmt.Errorf("必须指定 -c 载体文本与 -s 秘密文件")
			}
			out := output
			if out == "" {
				out = carrier + ".steg.md"
			}
			res, err := steg.EmbedText(carrier, out, secret, steg.Options{Password: password})
			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 零宽隐写完成: %s -> %s\n", res.Name, out)
				fmt.Printf("[!] 注意：请保持该文件以 UTF-8 保存，避免零宽字符被编辑器清除\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&carrier, "carrier", "c", "", "文本载体 (TXT/MD)")
	cmd.Flags().StringVarP(&secret, "secret", "s", "", "秘密文件")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出文本")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "加密密码")
	return cmd
}

func newZWDecodeCmd() *cobra.Command {
	var (
		input  string
		output string
		pass   string
	)
	cmd := &cobra.Command{
		Use:   "decode -i <文本> -o <输出目录> [-p <密码>]",
		Short: "从文本中提取零宽字符隐写秘密",
		Example: `  steggo zerowidth decode -i note.steg.md -o ./out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if input == "" {
				return fmt.Errorf("必须指定 -i 文本载体")
			}
			outDir := output
			if outDir == "" {
				outDir = "./extracted"
			}
			res, err := steg.ExtractText(input, outDir, password)
			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 提取完成: %s -> %s/\n", res.Name, outDir)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "文本载体")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出目录")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "解密密码")
	return cmd
}
