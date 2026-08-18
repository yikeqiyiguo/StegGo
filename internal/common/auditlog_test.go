package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuditChainHashChain 日志哈希链：写入→读取→校验通过；篡改→校验失败。
func TestAuditChainHashChain(t *testing.T) {
	dir := t.TempDir()
	al, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := al.Log(AuditEntry{
			Action: "hide",
			Target: "cover.png",
			Result: "ok",
			Detail: "algo=lsb",
			Hash:   "abc123",
		}); err != nil {
			t.Fatalf("Log %d: %v", i, err)
		}
	}
	if err := al.Close(); err != nil {
		t.Fatal(err)
	}

	// 重新打开读取
	al2, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer al2.Close()
	entries, err := al2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("记录数应为 5，实际 %d", len(entries))
	}
	for i, e := range entries {
		if e.Chain == "" {
			t.Fatalf("第 %d 条无哈希链", i)
		}
	}
	if err := VerifyChain(entries); err != nil {
		t.Fatalf("VerifyChain 应通过: %v", err)
	}

	// 篡改第一条 detail，链条应断裂
	entries[1].Detail = "tampered!"
	if err := VerifyChain(entries); err == nil {
		t.Fatal("篡改记录后校验应失败")
	}
}

// TestAuditLogAppendPreservesChain 追加模式保持链一致性。
func TestAuditLogAppendPreservesChain(t *testing.T) {
	dir := t.TempDir()
	al, _ := NewAuditLogger(dir)
	_ = al.Log(AuditEntry{Action: "a", Result: "ok"})
	_ = al.Close()

	al2, _ := NewAuditLogger(dir) // 重新打开（模拟进程重启）
	_ = al2.Log(AuditEntry{Action: "b", Result: "ok"})
	_ = al2.Close()

	al3, _ := NewAuditLogger(dir)
	defer al3.Close()
	entries, _ := al3.ReadAll()
	if len(entries) != 2 {
		t.Fatalf("记录数应为 2，实际 %d", len(entries))
	}
	if err := VerifyChain(entries); err != nil {
		t.Fatalf("跨进程追加后校验失败: %v", err)
	}
}

// TestWriteAuditPDF PDF 台账导出：文件头正确、含总哈希、可被后续校验。
func TestWriteAuditPDF(t *testing.T) {
	entries := []AuditEntry{
		{Time: "2026-08-14T10:00:00+08:00", Action: "hide", Target: "a.png", Result: "ok", Detail: "algo=dct", Hash: strings.Repeat("a", 64), Chain: strings.Repeat("b", 64)},
		{Time: "2026-08-14T10:01:00+08:00", Action: "extract", Target: "a.png", Result: "ok", Detail: "secret.txt", Hash: strings.Repeat("c", 64), Chain: strings.Repeat("d", 64)},
	}
	out := filepath.Join(t.TempDir(), "ledger.pdf")
	sum, err := WriteAuditPDF(entries, out, "StegGo 审计台账")
	if err != nil {
		t.Fatalf("WriteAuditPDF: %v", err)
	}
	if len(sum) != 64 {
		t.Fatalf("总哈希长度应为 64，实际 %d", len(sum))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-1.4") {
		t.Fatalf("PDF 文件头错误: %q", string(data[:16]))
	}
	if !strings.Contains(string(data), "%%EOF") {
		t.Fatal("PDF 缺少 EOF 标记")
	}
	if !strings.Contains(string(data), "ledger verify") {
		t.Fatal("PDF 应包含校验说明")
	}
}

// TestWriteAuditPDFEmpty 空台账导出不崩溃。
func TestWriteAuditPDFEmpty(t *testing.T) {
	out := filepath.Join(t.TempDir(), "empty.pdf")
	if _, err := WriteAuditPDF(nil, out, "空台账"); err != nil {
		t.Fatalf("空台账导出失败: %v", err)
	}
	if _, err := os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
}
