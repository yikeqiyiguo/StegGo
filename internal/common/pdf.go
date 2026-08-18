package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

// 手写最小 PDF 1.4 writer（无外部依赖），用于导出审计台账。
// 中文等非 Latin-1 字符统一转义为 \uXXXX，保证任何 PDF 阅读器均可打开。

const (
	pdfPageW  = 595 // A4 宽（pt）
	pdfPageH  = 842 // A4 高（pt）
	pdfMargin = 40
	pdfLineH  = 13
	pdfHeadH  = 4 // 页眉占 4 行（标题/生成时间/分隔）
	pdfMaxRow = (pdfPageH - pdfMargin*2 - pdfHeadH*pdfLineH) / pdfLineH
)

// pdfEscape 转义 PDF 字符串字面量（非 ASCII 转义为 \uXXXX）。
func pdfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r < 32 || r > 126 {
				fmt.Fprintf(&b, "\\u%04X", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// WriteAuditPDF 将审计记录导出为 PDF 台账，返回台账总哈希（十六进制）。
// 总哈希为全部规范化行文本的 SHA256，可独立校验导出内容完整性。
func WriteAuditPDF(entries []AuditEntry, outPath, title string) (string, error) {
	if title == "" {
		title = "StegGo 审计台账"
	}
	// 1. 生成行文本并计算总哈希
	lines := []string{
		"标题: " + title,
		"生成时间: " + time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf("记录总数: %d", len(entries)),
	}
	if len(entries) > 0 {
		lines = append(lines,
			"起始时间: "+entries[0].Time,
			"结束时间: "+entries[len(entries)-1].Time,
		)
	}
	lines = append(lines, "")
	for i, e := range entries {
		lines = append(lines, fmt.Sprintf("[%04d] %s | %s | %s | %s | %s | H:%s | C:%s",
			i+1, e.Time, e.Action, e.Target, e.Result, e.Detail,
			shortHash(e.Hash), shortHash(e.Chain)))
	}
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{0x0A})
	}
	sum := hex.EncodeToString(h.Sum(nil))
	lines = append(lines, "", "台账总哈希: "+sum, "校验方式: steggo ledger verify")

	// 2. 分页
	pages := (len(lines) + pdfMaxRow - 1) / pdfMaxRow
	if pages == 0 {
		pages = 1
	}

	// 3. 内容流 + 页对象（对象编号：1 Catalog, 2 Pages, 4 Font, 5.. 页/内容交错）
	var objs []string
	objs = append(objs,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [")
	kids := make([]string, 0, pages)
	for p := 0; p < pages; p++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 5+p*2))
	}
	objs = append(objs, strings.Join(kids, " ")+" ] /Count "+fmt.Sprintf("%d", pages)+" >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	for p := 0; p < pages; p++ {
		var cbuf bytes.Buffer
		if p == 0 {
			cbuf.WriteString("BT /F1 13 Tf 40 " + fmt.Sprintf("%d", pdfPageH-pdfMargin) + " Td (" +
				pdfEscape(title+"（StegGo V2.2）") + ") Tj ET\n")
			cbuf.WriteString("BT /F1 8 Tf 40 " + fmt.Sprintf("%d", pdfPageH-pdfMargin-16) + " Td (" +
				pdfEscape("生成时间: "+time.Now().Format("2006-01-02 15:04:05")) + ") Tj ET\n")
		}
		start := p * pdfMaxRow
		end := start + pdfMaxRow
		if end > len(lines) {
			end = len(lines)
		}
		y := pdfPageH - pdfMargin - pdfHeadH*pdfLineH
		for _, l := range lines[start:end] {
			cbuf.WriteString("BT /F1 8 Tf " + fmt.Sprintf("%d %d Td ", pdfMargin, y) +
				"(" + pdfEscape(l) + ") Tj ET\n")
			y -= pdfLineH
		}
		cbuf.WriteString("BT /F1 8 Tf 40 30 Td (" +
			fmt.Sprintf("第 %d/%d 页", p+1, pages) + ") Tj ET\n")

		pageObj := 5 + p*2
		contentObj := pageObj + 1
		objs = append(objs,
			fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] "+
				"/Resources << /Font << /F1 4 0 R >> >> /Contents %d 0 R >>\nendobj\n",
				pageObj, pdfPageW, pdfPageH, contentObj))
		objs = append(objs,
			fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n", contentObj, cbuf.Len())+
				cbuf.String()+"endstream\nendobj\n")
	}

	// 4. 写文件（含 xref 表）
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objs))
	for _, o := range objs {
		offsets = append(offsets, buf.Len())
		buf.WriteString(o)
	}
	xrefPos := buf.Len()
	buf.WriteString("xref\n0 " + fmt.Sprintf("%d", len(objs)+1) + "\n")
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	maxID := 5 + (pages-1)*2 + 1
	buf.WriteString("trailer\n<< /Size " + fmt.Sprintf("%d", maxID+1) + " /Root 1 0 R >>\n")
	buf.WriteString("startxref\n" + fmt.Sprintf("%d", xrefPos) + "\n%%EOF\n")

	if err := os.WriteFile(outPath, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	return sum, nil
}

// shortHash 取哈希前 12 位用于展示。
func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
