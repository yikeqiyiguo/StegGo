package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"steggo/internal/service"
)

func newExtractCmd() *cobra.Command {
	var (
		carrierPath string
		output      string
		pass        string
		keyfile     string
		useMachine  bool
		algorithm   string
	)
	cmd := &cobra.Command{
		Use:     "extract -c <载体> [-o <输出目录>] [-p <密码>]",
		Aliases: []string{"reveal"},
		Short:   "从载体提取加密的秘密并还原",
		Long: `从隐写载体中提取并解密还原原始文件（兼容 StegGo V1 旧版载体与 V2 全部算法）。

自动扫描算法参数（LSB 1-4 位深 / DCT / DWT / 自适应），无需手动指定；
V1.0 载体自动走兼容路径提取。

安全特性：
  - SHA256 完整性校验：载体被篡改立即报错
  - 密码错误/数据损坏返回统一错误，不泄露内部细节
  - 可否认载体：真实密码还原真实文件，诱饵密码还原诱饵`,
		Example: `  steggo extract -c cover.png.steg.png -p 密码
  steggo extract -c cover.png.steg.png -o ./out -p 密码
  steggo extract -c carrier.png --keyfile key.bin -p 密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if carrierPath == "" {
				return fmt.Errorf("必须指定 -c 载体")
			}
			outDir := output
			if outDir == "" {
				outDir = "./extracted"
			}
			svc := service.New()
			opt := service.Options{
				CarrierPath: carrierPath,
				OutputPath:  outDir,
				Password:    password,
				Algorithm:   algorithm,
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
			res, err := svc.Extract(opt)
			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 提取完成: %s (%d B) -> %s/\n", res.Name, res.Size, outDir)
				if res.IsDir {
					fmt.Println("[+] 已还原目录结构")
				}
				if res.Deniable {
					fmt.Printf("[+] 可否认命中区=%s\n", res.Region)
				}
				if res.V1Compat {
					fmt.Println("[+] V1.0 兼容路径")
				} else {
					fmt.Printf("[+] 算法=%s 位深=%d\n", res.Algorithm, res.BitDepth)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&carrierPath, "carrier", "c", "", "隐写载体文件")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出目录（默认 ./extracted）")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "解密密码（不传则交互输入）")
	cmd.Flags().StringVar(&keyfile, "keyfile", "", "密钥文件（三因子）")
	cmd.Flags().BoolVar(&useMachine, "machine", false, "绑定本机指纹（三因子）")
	cmd.Flags().StringVar(&algorithm, "algorithm", "", "指定算法（优先尝试；不传则自动扫描）")
	return cmd
}
