package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"steggo/internal/common"
)

// AuditReport 一次审计报告的内容。
type AuditReport struct {
	Time    string
	Action  string
	Results []*Result
}

// WriteAuditReport 将批量/分权结果汇总写入可读文本报告。
// 返回报告文件路径。
func WriteAuditReport(dir, action string, results []*Result) (string, error) {
	if err := common.EnsureDir(dir); err != nil {
		return "", err
	}
	ok, fail := batchSummary(results)
	now := time.Now()
	path := filepath.Join(dir, fmt.Sprintf("steggo_report_%s.txt", now.Format("20060102_150405")))

	var sb strings.Builder
	sb.WriteString("========================================\n")
	sb.WriteString("  StegGo V2.0 审计报告\n")
	sb.WriteString("========================================\n")
	sb.WriteString("时间:   " + now.Format("2006-01-02 15:04:05") + "\n")
	sb.WriteString("操作:   " + action + "\n")
	sb.WriteString("成功:   " + fmt.Sprintf("%d", ok) + "\n")
	sb.WriteString("失败:   " + fmt.Sprintf("%d", fail) + "\n")
	sb.WriteString("----------------------------------------\n")
	for i, r := range results {
		if r == nil {
			continue
		}
		status := "OK  "
		if r.Size < 0 {
			status = "FAIL"
		}
		line := fmt.Sprintf("%2d. [%s] %s", i+1, status, r.Name)
		if r.Size >= 0 {
			line += fmt.Sprintf("  size=%d", r.Size)
			if r.Algorithm != "" {
				line += "  algo=" + r.Algorithm
			}
			if r.IsDir {
				line += "  dir"
			}
			if r.V1Compat {
				line += "  v1-compat"
			}
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("========================================\n")
	return path, os.WriteFile(path, []byte(sb.String()), 0o644)
}

// WriteExtractionReport 生成单次提取的审计报告（含载荷元信息）。
func (s *Service) WriteExtractionReport(res *Result) (string, error) {
	if res == nil {
		return "", fmt.Errorf("无结果可写")
	}
	dir := res.OutPath
	if dir == "" {
		dir = "."
	}
	return WriteAuditReport(dir, "extract", []*Result{res})
}
