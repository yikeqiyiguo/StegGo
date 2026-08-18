package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"steggo/internal/carrier"
	"steggo/internal/service"
)

// splitCarriers 拆分逗号分隔的载体列表并去除空白。
func splitCarriers(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// newNestedCmd 套娃（递归嵌套）命令。
// 嵌套 = 逐层调用 service.Embed：每层把上一层输出文件作为秘密加密嵌入，
// 因此每层都是完整加密载荷，提取时逐层解密剥离。
func newNestedCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nested",
		Short: "套娃：多层载体递归嵌套隐写",
		Long: `套娃隐写将秘密逐层嵌入多个载体（每层独立加密）：
  载体列表顺序 = 内层→外层：第一个载体最先嵌入秘密，
  最后一个载体是最终输出。提取时从最外层逐层剥离。`,
	}
	embed := &cobra.Command{
		Use:   "embed -c <载体1,载体2,...> -s <秘密> [-o <输出目录>] [-p <密码>]",
		Short: "递归嵌套嵌入",
		Example: `  steggo nested embed -c in1.png,in2.png -s secret.txt -p 密码
  steggo nested embed -c in1.png,in2.png,in3.wav -s secret.txt -o ./out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			carrierList, _ := cmd.Flags().GetString("carrier")
			secret, _ := cmd.Flags().GetString("secret")
			outDir, _ := cmd.Flags().GetString("output")
			pass, _ := cmd.Flags().GetString("password")
			bits, _ := cmd.Flags().GetInt("bits")
			if carrierList == "" || secret == "" {
				return fmt.Errorf("必须指定 -c 载体列表（逗号分隔）与 -s 秘密")
			}
			carriers := splitCarriers(carrierList)
			if len(carriers) < 2 {
				return fmt.Errorf("套娃至少需要两层载体")
			}
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if outDir == "" {
				outDir = "."
			}
			svc := service.New()
			cur := secret
			for i, c := range carriers {
				out := filepath.Join(outDir, fmt.Sprintf("nested_%02d%s", i+1, filepath.Ext(c)))
				opt := service.Options{
					CarrierPath: c,
					SecretPath:  cur,
					OutputPath:  out,
					Password:    password,
					Algorithm:   "lsb",
					BitDepth:    bits,
				}
				if _, err := svc.Embed(opt); err != nil {
					return fmt.Errorf("第 %d 层嵌入失败: %w", i+1, err)
				}
				cur = out
			}
			if !quiet {
				fmt.Printf("[OK] 套娃嵌入完成: %d 层 -> %s\n", len(carriers), cur)
			}
			return nil
		},
	}
	embed.Flags().StringP("carrier", "c", "", "载体列表，逗号分隔（内层->外层）")
	embed.Flags().StringP("secret", "s", "", "秘密文件")
	embed.Flags().StringP("output", "o", "", "输出目录（默认当前目录）")
	embed.Flags().StringP("password", "p", "", "加密密码")
	embed.Flags().IntP("bits", "b", 1, "每通道嵌入位数")

	extract := &cobra.Command{
		Use:     "extract -c <最外层载体> -d <层数> [-o <输出目录>] [-p <密码>]",
		Short:   "从最外层逐层剥离提取",
		Example: `  steggo nested extract -c nested_02.png -d 2 -p 密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			outer, _ := cmd.Flags().GetString("carrier")
			depth, _ := cmd.Flags().GetInt("depth")
			outDir, _ := cmd.Flags().GetString("output")
			pass, _ := cmd.Flags().GetString("password")
			if outer == "" || depth < 2 {
				return fmt.Errorf("必须指定 -c 最外层载体与 -d 层数(>=2)")
			}
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if outDir == "" {
				outDir = "."
			}
			svc := service.New()
			cur := outer
			for i := 0; i < depth; i++ {
				layerDir := filepath.Join(outDir, fmt.Sprintf("layer_%d", depth-i))
				res, err := svc.Extract(service.Options{
					CarrierPath: cur,
					OutputPath:  layerDir,
					Password:    password,
				})
				if err != nil {
					return fmt.Errorf("第 %d 层提取失败: %w", i+1, err)
				}
				if i == depth-1 {
					if !quiet {
						fmt.Printf("[OK] 套娃提取完成: %d 层 -> %s (%s)\n", depth, layerDir, res.Name)
					}
					return nil
				}
				cur = filepath.Join(layerDir, res.Name)
			}
			return nil
		},
	}
	extract.Flags().StringP("carrier", "c", "", "最外层载体文件")
	extract.Flags().IntP("depth", "d", 0, "嵌套层数（>=2）")
	extract.Flags().StringP("output", "o", "", "输出目录（默认当前目录）")
	extract.Flags().StringP("password", "p", "", "加密密码")

	expand := &cobra.Command{
		Use:   "expand -c <最外层载体> [-o <输出目录>] [-p <密码>]",
		Short: "一键展开：自动探测嵌套深度并导出全部层级",
		Long: `自动从最外层逐层剥离，直到最内层秘密，无需手动指定层数：
  - 每层写入 layer_01/、layer_02/ ... 子目录，含全部内嵌文件；
  - 任一层不再是可识别载体（或提取失败）即视为最内层并停止；
  - 密码错误或文件并非隐写载体时明确报错。`,
		Example: `  steggo nested expand -c nested_03.png -p 密码 -o ./expanded`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			outer, _ := cmd.Flags().GetString("carrier")
			outDir, _ := cmd.Flags().GetString("output")
			pass, _ := cmd.Flags().GetString("password")
			limit, _ := cmd.Flags().GetInt("limit")
			if outer == "" {
				return fmt.Errorf("必须指定 -c 最外层载体")
			}
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if outDir == "" {
				outDir = "."
			}
			svc := service.New()
			cur := outer
			layers := 0
			for layers < limit {
				layerDir := filepath.Join(outDir, fmt.Sprintf("layer_%02d", layers+1))
				res, err := svc.Extract(service.Options{
					CarrierPath: cur,
					OutputPath:  layerDir,
					Password:    password,
				})
				if err != nil {
					// 当前文件非嵌套载体（或已是最终秘密），展开结束。
					break
				}
				layers++
				if !quiet {
					fmt.Printf("[+] 第 %d 层: %s (%d B, %s)\n", layers, res.Name, res.Size, res.Algorithm)
				}
				if res.IsDir {
					break // 目录即最内层
				}
				next := filepath.Join(layerDir, res.Name)
				if _, kerr := carrier.DetectKind(next); kerr != nil {
					break // 解出的文件不再是可识别载体 = 最内层秘密
				}
				cur = next
			}
			if layers == 0 {
				return fmt.Errorf("未从 %s 中提取到任何有效载荷（密码错误或文件并非隐写载体）", outer)
			}
			if !quiet {
				fmt.Printf("[OK] 一键展开完成: 共 %d 层 -> %s\n", layers, outDir)
			}
			return nil
		},
	}
	expand.Flags().StringP("carrier", "c", "", "最外层载体文件")
	expand.Flags().StringP("output", "o", "", "输出目录（默认当前目录）")
	expand.Flags().StringP("password", "p", "", "加密密码")
	expand.Flags().IntP("limit", "l", 32, "最大展开层数（防止死循环）")

	root.AddCommand(embed, extract, expand)
	return root
}
