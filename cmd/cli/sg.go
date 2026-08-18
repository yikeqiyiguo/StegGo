package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"steggo/internal/service"
)

// newSGCmd .sg 加密容器命令。
// 将整个载体文件 AES-256-GCM / SM4-GCM 加密打包为独立容器，
// 防止载体被直接篡改、扫描或逆向分析。
func newSGCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sg",
		Short: "载体加密容器：将载体整体加密打包为 .sg",
		Long: `将整个载体文件加密打包为独立 .sg 容器（AES-256-GCM 默认 / SM4 国密可选），
防止载体被直接篡改、扫描或逆向分析。需要使用时再解密还原。`,
	}
	root.AddCommand(newSGCreateCmd(), newSGOpenCmd())
	return root
}

func newSGCreateCmd() *cobra.Command {
	var (
		in     string
		out    string
		pass   string
		useSM4 bool
	)
	cmd := &cobra.Command{
		Use:   "create -i <载体> -p <密码> [-o <输出.sg>]",
		Short: "将载体加密打包为 .sg 容器",
		Example: `  steggo sg create -i cover.png -p 密码 -o cover.png.sg
  steggo sg create -i cover.png -p 密码 --sm4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if in == "" {
				return fmt.Errorf("必须指定 -i 输入文件")
			}
			password, err := resolvePassword(pass, "输入容器密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			res, err := service.New().ContainerEncrypt(in, out, password, useSM4)
			if err != nil {
				return err
			}
			if !quiet {
				algo := "AES-256-GCM"
				if res.UseSM4 {
					algo = "SM4-GCM"
				}
				fmt.Printf("[OK] 容器已创建: %s (%d B, %s)\n", res.OutPath, res.Size, algo)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&in, "input", "i", "", "载体文件路径")
	cmd.Flags().StringVarP(&out, "output", "o", "", "输出 .sg 路径（默认 <输入>.sg）")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "容器密码（不传则交互输入）")
	cmd.Flags().BoolVar(&useSM4, "sm4", false, "使用 SM4-GCM 国密算法（默认 AES-256-GCM）")
	return cmd
}

func newSGOpenCmd() *cobra.Command {
	var (
		in   string
		out  string
		pass string
	)
	cmd := &cobra.Command{
		Use:     "open -i <容器.sg> -p <密码> [-o <输出>]",
		Short:   "解密 .sg 容器还原原始载体",
		Example: `  steggo sg open -i cover.png.sg -p 密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if in == "" {
				return fmt.Errorf("必须指定 -i 容器文件")
			}
			password, err := resolvePassword(pass, "输入容器密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			res, err := service.New().ContainerDecrypt(in, out, password)
			if err != nil {
				return err
			}
			if !quiet {
				algo := "AES-256-GCM"
				if res.UseSM4 {
					algo = "SM4-GCM"
				}
				fmt.Printf("[OK] 容器已解密: %s (%d B, %s)\n", res.OutPath, res.Size, algo)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&in, "input", "i", "", ".sg 容器路径")
	cmd.Flags().StringVarP(&out, "output", "o", "", "输出路径（默认还原原始文件名）")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "容器密码（不传则交互输入）")
	return cmd
}
