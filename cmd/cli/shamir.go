package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"steggo/pkg/steg"
)

func newShamirCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shamir",
		Short: "Shamir 门限分片 (k, n)",
		Long: `将秘密拆分为 n 个分片，任意凑齐 k 个即可完整恢复。
少于 k 个分片得不到任何明文信息。

典型用途：把分片分发给多人/多载体，实现权限分离。`,
	}
	cmd.AddCommand(newShamirSplitCmd(), newShamirRecoverCmd())
	return cmd
}

func newShamirSplitCmd() *cobra.Command {
	var (
		input string
		total int
		need  int
		output string
	)
	cmd := &cobra.Command{
		Use:   "split -i <秘密文件> -n <总片数> -k <门限> [-o <目录>]",
		Short: "拆分秘密为 (k, n) 门限分片",
		Example: `  steggo shamir split -i secret.zip -n 5 -k 3 -o ./shares`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if input == "" || total <= 0 || need <= 0 {
				return fmt.Errorf("必须指定 -i 文件、-n 总片数、-k 门限")
			}
			data, err := os.ReadFile(input)
			if err != nil {
				return err
			}
			shares, err := steg.SplitSecret(data, total, need)
			if err != nil {
				return err
			}
			if output == "" {
				output = "./shares"
			}
			if err := os.MkdirAll(output, 0700); err != nil {
				return err
			}
			for i, s := range shares {
				name := filepath.Join(output, fmt.Sprintf("share_%02d.bin", i+1))
				if err := os.WriteFile(name, s, 0600); err != nil {
					return err
				}
			}
			if !quiet {
				fmt.Printf("[OK] 已生成 %d 个分片 (%d/%d 门限) -> %s/\n", total, need, total, output)
				fmt.Printf("[!] 请将分片分开存放；至少需要 %d 个分片才能恢复\n", need)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "要拆分的秘密文件")
	cmd.Flags().IntVarP(&total, "total", "n", 0, "分片总数")
	cmd.Flags().IntVarP(&need, "need", "k", 0, "恢复所需门限数")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出目录")
	return cmd
}

func newShamirRecoverCmd() *cobra.Command {
	var (
		dir   string
		need  int
		output string
	)
	cmd := &cobra.Command{
		Use:   "recover -d <分片目录> -k <门限> -o <输出文件>",
		Short: "从分片恢复秘密（任意 k 个分片）",
		Example: `  steggo shamir recover -d ./shares -k 3 -o secret.zip`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if dir == "" || need <= 0 || output == "" {
				return fmt.Errorf("必须指定 -d 分片目录、-k 门限、-o 输出文件")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			var names []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasPrefix(e.Name(), "share_") {
					names = append(names, e.Name())
				}
			}
			if len(names) < need {
				return fmt.Errorf("分片不足：目录中有 %d 个分片，需要 %d 个", len(names), need)
			}
			sort.Strings(names)
			var shares [][]byte
			for _, n := range names[:need] {
				data, err := os.ReadFile(filepath.Join(dir, n))
				if err != nil {
					return err
				}
				shares = append(shares, data)
			}
			plain, err := steg.RecoverSecret(shares, need)
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, plain, 0600); err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("[OK] 使用 %d 个分片恢复成功 -> %s (%d B)\n", need, output, len(plain))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "分片目录")
	cmd.Flags().IntVarP(&need, "need", "k", 0, "恢复所需门限数")
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出文件")
	return cmd
}
