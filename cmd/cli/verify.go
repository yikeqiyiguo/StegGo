package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var (
		file string
		hash string
	)
	cmd := &cobra.Command{
		Use:   "verify -f <载体文件> [-h <哈希值或 .sha256 文件>]",
		Short: "载体完整性校验 (SHA256)",
		Long: `计算载体文件 SHA256 并与期望值比对，用于确认载体未被篡改
（隐写数据自带 AES-GCM 与 SHA256 双重校验，此命令用于人工核验）。`,
		Example: `  steggo verify -f out.png -h 6f5c...（期望哈希）
  steggo verify -f out.png -h out.png.sha256`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("必须指定 -f 文件")
			}
			actual, err := hashFile(file)
			if err != nil {
				return err
			}
			fmt.Printf("文件    : %s\n", file)
			fmt.Printf("SHA256  : %s\n", actual)
			if hash == "" {
				fmt.Println("[i] 未提供期望哈希，仅输出计算结果")
				return nil
			}
			expected := hash
			if data, err := os.ReadFile(hash); err == nil {
				// hash 参数指向一个 .sha256 文件
				expected = parseHashFile(string(data))
			}
			expected = normalizeHash(expected)
			if expected == actual {
				fmt.Println("[OK] 哈希匹配：载体完整未被篡改")
			} else {
				fmt.Println("[FAIL] 哈希不匹配：载体可能已被修改！")
				return fmt.Errorf("哈希校验失败")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "待校验文件")
	cmd.Flags().StringVarP(&hash, "hash", "h", "", "期望 SHA256 或 .sha256 文件路径")
	return cmd
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func parseHashFile(content string) string {
	fields := splitFields(content)
	for _, f := range fields {
		if len(f) == 64 && isHex(f) {
			return f
		}
	}
	return ""
}

func splitFields(s string) []string {
	var out []string
	var cur []rune
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func normalizeHash(s string) string {
	for _, c := range s {
		if c < '0' || (c > '9' && c < 'a') || (c > 'f' && c < 'A') || (c > 'F' && c <= 'z') {
			continue
		}
	}
	return s
}

var _ = filepath.Base
