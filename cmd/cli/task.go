package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"steggo/internal/scheduler"
)

// newTaskCmd 任务清单批量执行命令。
func newTaskCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "task run -f <tasks.csv|tasks.txt>",
		Short: "按 TXT/CSV 任务清单批量执行隐写/提取",
		Long: `按任务清单批量执行隐写/提取/容器操作。

CSV 表头: action,carrier,secret,password,algorithm,bits,sm4,output
TXT 格式: action=hide carrier="cover.png" secret=msg.txt password=pass algorithm=lsb bits=2 output=out.steg

action 支持: hide / extract / sg-create / sg-open
# 开头的行视为注释；缺 password 的任务会失败并在汇总中报告。`,
		Example: `  steggo task run -f tasks.csv
  steggo task run -f tasks.txt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if file == "" {
				return fmt.Errorf("必须指定 -f 任务清单文件")
			}
			abs, err := filepath.Abs(file)
			if err != nil {
				return err
			}
			tasks, err := scheduler.ParseFile(abs)
			if err != nil {
				return err
			}
			sum := (&scheduler.Runner{}).Run(tasks)
			if !quiet {
				fmt.Printf("[汇总] 共 %d 条，成功 %d，失败 %d\n", sum.Total, sum.Success, sum.Failed)
				for _, e := range sum.Errors {
					fmt.Fprintln(os.Stderr, "  ✗ "+e)
				}
			}
			if sum.Failed > 0 {
				return fmt.Errorf("%d/%d 条任务失败", sum.Failed, sum.Total)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "任务清单文件（.csv 或 .txt）")
	return cmd
}
