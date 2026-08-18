package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// newScheduleCmd 定时调度命令（Linux crontab 自动解密备份）。
func newScheduleCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "schedule",
		Short: "定时调度：生成 Linux crontab 自动解密备份配置",
		Long: `为 Linux crontab 生成定时自动解密备份配置。
安全提示：任务中会记录解密密码，请使用 --password-file 指定密码文件
（文件权限设为 600 并放置于仅自己可读的位置），避免密码明文出现在 crontab 中。`,
	}
	root.AddCommand(newScheduleCronCmd())
	return root
}

func newScheduleCronCmd() *cobra.Command {
	var (
		carrier      string
		password     string
		passwordFile string
		output       string
		everyMin     int
		install      bool
	)
	cmd := &cobra.Command{
		Use:   "cron --carrier <载体> --output <目录> [--password <密码>|--password-file <文件>]",
		Short: "生成 crontab 定时解密配置（默认每 30 分钟）",
		Example: `  # 仅打印配置
  steggo schedule cron --carrier /data/backup.steg --output /data/restore --password-file /root/.steggo_pw

  # 直接安装到当前用户 crontab（Linux）
  steggo schedule cron --carrier /data/backup.steg --output /data/restore --password-file /root/.steggo_pw --install`,
		RunE: func(cmd *cobra.Command, args []string) error {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if carrier == "" || output == "" {
				return fmt.Errorf("必须指定 --carrier 与 --output")
			}
			if password == "" && passwordFile == "" {
				return fmt.Errorf("必须指定 --password 或 --password-file 之一")
			}
			if passwordFile != "" {
				abs, err := filepath.Abs(passwordFile)
				if err != nil {
					return err
				}
				passwordFile = abs
				// 校验密码文件可读
				if _, err := os.Stat(passwordFile); err != nil {
					return fmt.Errorf("密码文件不可读: %w", err)
				}
			}
			if everyMin <= 0 {
				everyMin = 30
			}
			if everyMin < 1 || everyMin > 1440 {
				return fmt.Errorf("间隔分钟数需在 1-1440 之间")
			}
			absCarrier, _ := filepath.Abs(carrier)
			absOut, _ := filepath.Abs(output)

			var secretArg string
			if passwordFile != "" {
				secretArg = "--password-file " + shellQuote(passwordFile)
			} else {
				secretArg = "--password " + shellQuote(password)
			}
			bin, err := os.Executable()
			if err != nil {
				bin = "steggo"
			}
			line := fmt.Sprintf("*/%d * * * * %s extract -c %s %s -o %s >> %s 2>&1",
				everyMin, shellQuote(bin), shellQuote(absCarrier), secretArg,
				shellQuote(absOut), shellQuote(filepath.Join(absOut, "cron.log")))

			if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
				if install {
					return fmt.Errorf("--install 仅支持 Linux/macOS（当前 %s），请手动将以下配置加入 crontab", runtime.GOOS)
				}
			}

			if !quiet {
				fmt.Println("# StegGo 定时自动解密备份（每 " + fmt.Sprintf("%d", everyMin) + " 分钟）")
				fmt.Println(line)
				fmt.Println("# 安装: crontab -e 粘贴以上行，或使用 --install")
			}
			if install {
				if err := installCrontab(line); err != nil {
					return err
				}
				if !quiet {
					fmt.Println("[OK] 已安装到当前用户 crontab")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&carrier, "carrier", "", "待解密载体文件路径")
	cmd.Flags().StringVar(&password, "password", "", "解密密码（明文，不推荐）")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "密码文件路径（推荐）")
	cmd.Flags().StringVar(&output, "output", "", "解密输出目录")
	cmd.Flags().IntVar(&everyMin, "every", 30, "执行间隔（分钟，1-1440）")
	cmd.Flags().BoolVar(&install, "install", false, "直接安装到 crontab（Linux/macOS）")
	return cmd
}

// shellQuote 简单 shell 单引号转义。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// installCrontab 将一行配置追加到当前用户 crontab（保留原有条目）。
func installCrontab(line string) error {
	out, err := exec.Command("crontab", "-l").Output()
	existing := strings.TrimSpace(string(out))
	if err != nil && existing == "" {
		// 无现有 crontab 视为空
		existing = ""
	}
	content := existing
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		content += "\n"
	}
	content += line + "\n"
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
