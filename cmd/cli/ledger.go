package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"steggo/internal/common"
)

// newLedgerCmd 审计台账命令：导出 PDF / 校验哈希链。
func newLedgerCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ledger",
		Short: "审计台账：导出 PDF 完整台账 / 校验哈希链防篡改",
		Long: `操作日志导出 PDF 完整审计台账（带哈希链防篡改），并支持重放校验：
任何历史记录被修改都会导致后续哈希链全部断裂。`,
	}
	root.AddCommand(newLedgerExportCmd(), newLedgerVerifyCmd())
	return root
}

func newLedgerExportCmd() *cobra.Command {
	var (
		dir string
		out string
	)
	cmd := &cobra.Command{
		Use:     "export -o <台账.pdf>",
		Short:   "导出审计台账 PDF",
		Example: `  steggo ledger export -o audit-ledger.pdf`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			dir, err := resolveAuditDir(dir)
			if err != nil {
				return err
			}
			al, err := common.NewAuditLogger(dir)
			if err != nil {
				return fmt.Errorf("打开审计日志: %w", err)
			}
			defer al.Close()
			entries, err := al.ReadAll()
			if err != nil {
				return fmt.Errorf("读取审计日志: %w", err)
			}
			if out == "" {
				out = "steggo-audit-ledger.pdf"
			}
			sum, err := common.WriteAuditPDF(entries, out, "StegGo 审计台账")
			if err != nil {
				return fmt.Errorf("导出 PDF: %w", err)
			}
			if !quiet {
				fmt.Printf("[OK] 已导出 %d 条审计记录 -> %s\n", len(entries), out)
				fmt.Printf("台账总哈希: %s\n", sum)
				fmt.Println("校验方式: steggo ledger verify")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "审计日志目录（默认应用数据目录）")
	cmd.Flags().StringVarP(&out, "output", "o", "", "PDF 输出路径")
	return cmd
}

func newLedgerVerifyCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:     "verify",
		Short:   "校验审计日志哈希链完整性",
		Example: `  steggo ledger verify`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			dir, err := resolveAuditDir(dir)
			if err != nil {
				return err
			}
			al, err := common.NewAuditLogger(dir)
			if err != nil {
				return fmt.Errorf("打开审计日志: %w", err)
			}
			defer al.Close()
			entries, err := al.ReadAll()
			if err != nil {
				return fmt.Errorf("读取审计日志: %w", err)
			}
			if err := common.VerifyChain(entries); err != nil {
				return fmt.Errorf("校验失败: %w", err)
			}
			if !quiet {
				fmt.Printf("[OK] 哈希链校验通过：%d 条记录未被篡改\n", len(entries))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "审计日志目录（默认应用数据目录）")
	return cmd
}

// resolveAuditDir 解析审计目录（默认应用数据目录）。
func resolveAuditDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	return common.AppDataDir()
}
