package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"steggo/internal/crypto"
	"steggo/internal/service"
)

func newExtractCmd() *cobra.Command {
	var (
		carrierPath  string
		output       string
		pass         string
		keyfile      string
		useMachine   bool
		algorithm    string
		usbDir       string
		passwordFile string
		kyberPriv    string
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
  - 可否认载体：真实密码还原真实文件，诱饵密码还原诱饵
  - 后量子载荷需 --kyber-priv 私钥解封装；RS-ECC 载荷自动纠错（无需参数）`,
		Example: `  steggo extract -c cover.png.steg.png -p 密码
  steggo extract -c cover.png.steg.png -o ./out -p 密码
  steggo extract -c carrier.png --keyfile key.bin -p 密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePasswordEx(pass, passwordFile, "输入密码: ")
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
			if err := applyUSBKey(&opt, usbDir); err != nil {
				return err
			}
			if kyberPriv != "" {
				priv, kerr := readKyberKey(kyberPriv, crypto.KyberPrivKeySize, "私钥")
				if kerr != nil {
					return fmt.Errorf("读取后量子私钥: %w", kerr)
				}
				defer wipe(priv)
				opt.KyberPriv = priv
			}
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
				if res.Kyber {
					fmt.Println("[+] 后量子解密: ML-KEM-768 主密钥解封装成功")
				}
				if res.ECCLevel != "" {
					fmt.Printf("[+] RS-ECC 纠错: %s 级 | %d 块 | 修复 %d 符号 | 完好率 %.1f%%\n",
						res.ECCLevel, res.ECCBlocks, res.ECCCorrectedErrors, res.ECCRepairRate*100)
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
	cmd.Flags().StringVar(&usbDir, "usb", "", "USB 密钥盘目录（令牌+设备序列号绑定解锁）")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "从文件读取密码（首行，crontab 场景推荐）")
	cmd.Flags().StringVar(&kyberPriv, "kyber-priv", "", "后量子私钥文件（ML-KEM-768，解封装混合加密载荷）")
	return cmd
}
