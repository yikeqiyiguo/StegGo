package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"steggo/internal/common"
	"steggo/internal/crypto"
)

// newKyberCmd 后量子加密（ML-KEM-768）命令。
func newKyberCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kyber",
		Short: "后量子加密 ML-KEM-768：生成密钥对，供 --kyber-pub / --kyber-priv 使用",
		Long: `后量子加密（ML-KEM-768，NIST FIPS 203 / CRYSTALS-Kyber 标准版）。

使用方式：
  1. 接收方执行 steggo kyber keygen 生成密钥对；
  2. 将公钥文件分发给发送方；
  3. 发送方隐写时加 --kyber-pub <公钥文件>：随机 AES 主密钥加密明文，
     主密钥再用 ML-KEM-768 公钥封装后随密文保存（混合加密，前向安全）；
  4. 接收方提取时加 --kyber-priv <私钥文件> 即可解封装解密。

即使量子计算机破解对称密钥交换历史，也无法回退解密。`,
		Example: `  steggo kyber keygen -o pub.kyb -k priv.kyb
  steggo hide -c photo.png -s secret.txt --kyber-pub pub.kyb -p pass -o steg.png
  steggo extract -c steg.png --kyber-priv priv.kyb -p pass -o ./out`,
	}

	keygen := &cobra.Command{
		Use:   "keygen -o <公钥> -k <私钥>",
		Short: "生成 ML-KEM-768 后量子密钥对",
		RunE: func(cmd *cobra.Command, args []string) error {
			pubPath, _ := cmd.Flags().GetString("output")
			privPath, _ := cmd.Flags().GetString("priv")
			if pubPath == "" || privPath == "" {
				return fmt.Errorf("必须指定 -o 公钥路径与 -k 私钥路径")
			}
			wrap, err := crypto.GenerateKyberKeyPair()
			if err != nil {
				return fmt.Errorf("生成后量子密钥对: %w", err)
			}
			defer common.Wipe(wrap.PrivKey)
			if err := os.WriteFile(pubPath, wrap.PubKey, 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(privPath, wrap.PrivKey, 0o600); err != nil {
				return err
			}
			fmt.Printf("[OK] ML-KEM-768 密钥对已生成\n")
			fmt.Printf("  公钥: %s (%d B)  -> 分发给发送方\n", pubPath, len(wrap.PubKey))
			fmt.Printf("  私钥: %s (%d B)  -> 接收方自留，注意保密\n", privPath, len(wrap.PrivKey))
			fmt.Printf("  用法: hide --kyber-pub %s | extract --kyber-priv %s\n", pubPath, privPath)
			return nil
		},
	}
	keygen.Flags().StringP("output", "o", "", "公钥输出路径（如 pub.kyb）")
	keygen.Flags().StringP("priv", "k", "", "私钥输出路径（如 priv.kyb）")
	_ = keygen.MarkFlagRequired("output")
	_ = keygen.MarkFlagRequired("priv")

	// 可选：单独生成公钥/私钥文件到指定目录
	root.AddCommand(keygen)

	info := &cobra.Command{
		Use:   "info",
		Short: "显示后量子加密状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !crypto.KyberAvailable() {
				return fmt.Errorf("后量子 KEM 未注册")
			}
			kem, _ := crypto.NewKyberKEM()
			fmt.Printf("[+] 后量子加密可用: %s\n", kem.Name())
			fmt.Printf("  公钥 %d B / 私钥 %d B / 密文 %d B / 共享密钥 %d B\n",
				crypto.KyberPubKeySize, crypto.KyberPrivKeySize,
				crypto.KyberCipherSize, crypto.KyberSharedSize)
			fmt.Printf("  实现: Go 标准库 crypto/mlkem（FIPS 203，零外部依赖）\n")
			return nil
		},
	}
	root.AddCommand(info)
	return root
}

// readKyberKey 读取后量子密钥文件。
func readKyberKey(path string, want int, what string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if want > 0 && len(key) != want {
		return nil, fmt.Errorf("%s长度 %d != %d（文件可能损坏或非后量子密钥）", what, len(key), want)
	}
	return key, nil
}
