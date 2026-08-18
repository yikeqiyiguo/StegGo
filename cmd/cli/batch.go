package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"steggo/pkg/steg"
)

func newBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "批量隐写任务",
		Long:  `批量将秘密嵌入目录内全部支持载体，或批量从目录内载体提取秘密。`,
	}
	cmd.AddCommand(newBatchEmbedCmd(), newBatchExtractCmd())
	return cmd
}

func newBatchEmbedCmd() *cobra.Command {
	var (
		carrierDir string
		secret     string
		outputDir  string
		pass       string
		bits       int
		recursive  bool
		concurrent int
	)
	cmd := &cobra.Command{
		Use:     "embed -d <载体目录> -s <秘密> -o <输出目录> [-p <密码>]",
		Short:   "将秘密批量嵌入目录中的全部载体",
		Example: `  steggo batch embed -d ./carriers -s secret.txt -o ./out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if carrierDir == "" || secret == "" {
				return fmt.Errorf("必须指定 -d 载体目录与 -s 秘密文件")
			}
			opts := steg.BatchEmbedOptions{
				Options:     steg.Options{BitDepth: bits, Password: password},
				Concurrency: concurrent,
				IncludeDirs: recursive,
			}
			results, err := steg.BatchEmbed(context.Background(), carrierDir, outputDir, secret, opts)
			if err != nil {
				return err
			}
			var ok, fail int
			for _, r := range results {
				if r.Error != nil {
					fail++
					if !quiet {
						fmt.Printf("[ERR] %s: %v\n", r.Carrier, r.Error)
					}
				} else {
					ok++
				}
			}
			fmt.Printf("[OK] 批量嵌入完成: 成功 %d, 失败 %d\n", ok, fail)
			return nil
		},
	}
	cmd.Flags().StringVarP(&carrierDir, "dir", "d", "", "载体目录")
	cmd.Flags().StringVarP(&secret, "secret", "s", "", "秘密文件")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./batch-out", "输出目录")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "加密密码")
	cmd.Flags().IntVarP(&bits, "bits", "b", 2, "嵌入位数 (1-4)")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "递归子目录")
	cmd.Flags().IntVar(&concurrent, "concurrency", 4, "并发数")
	return cmd
}

func newBatchExtractCmd() *cobra.Command {
	var (
		carrierDir string
		outputDir  string
		pass       string
		recursive  bool
		concurrent int
	)
	cmd := &cobra.Command{
		Use:     "extract -d <载体目录> -o <输出目录> [-p <密码>]",
		Short:   "从目录中的全部载体批量提取秘密",
		Example: `  steggo batch extract -d ./out -o ./extracted`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			password, err := resolvePassword(pass, "输入密码: ")
			if err != nil {
				return err
			}
			defer wipe(password)
			if carrierDir == "" {
				return fmt.Errorf("必须指定 -d 载体目录")
			}
			opts := steg.BatchExtractOptions{Concurrency: concurrent, IncludeDirs: recursive}
			results, err := steg.BatchExtract(context.Background(), carrierDir, outputDir, password, opts)
			if err != nil {
				return err
			}
			var ok, fail int
			for _, r := range results {
				if r.Error != nil {
					fail++
					if !quiet {
						fmt.Printf("[ERR] %s: %v\n", r.Carrier, r.Error)
					}
				} else {
					ok++
				}
			}
			fmt.Printf("[OK] 批量提取完成: 成功 %d, 失败 %d -> %s\n", ok, fail, outputDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&carrierDir, "dir", "d", "", "载体目录")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./extracted", "输出目录")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "解密密码")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "递归子目录")
	cmd.Flags().IntVar(&concurrent, "concurrency", 4, "并发数")
	return cmd
}

var _ = filepath.Base
