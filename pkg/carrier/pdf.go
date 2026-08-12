package carrier

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

// PDF 载体：在最后一个 %%EOF 标记之前插入内部冗余数据流。
//
//	[payload][payloadLen uint64 BE 8B][tag 8B]   ← 插入在 %%EOF 之前
//
// 插入位置位于 xref 表之后、文件末尾之前，不影响任何对象偏移，
// PDF 渲染结构完全不变，阅读器正常打开。
var pdfTagV2 = []byte("STGPDFV2")
var pdfTagV1 = []byte("STGPDF01") // 早期版本兼容（文件末尾 EOF 追加）

// EmbedPDF 将载荷嵌入 PDF 内部冗余数据流。
func EmbedPDF(carrierPath, outputPath string, payload []byte) error {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return err
	}
	eofIdx := bytes.LastIndex(data, []byte("%%EOF"))
	if eofIdx < 0 {
		return errors.New("不是有效的 PDF 文件（缺少 %%EOF 标记）")
	}

	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	blob := make([]byte, 0, len(payload)+16)
	blob = append(blob, payload...)
	blob = append(blob, lenBuf[:]...)
	blob = append(blob, pdfTagV2...)

	out := make([]byte, 0, len(data)+len(blob))
	out = append(out, data[:eofIdx]...)
	out = append(out, blob...)
	out = append(out, data[eofIdx:]...)
	return os.WriteFile(outputPath, out, 0644)
}

// ExtractPDF 从 PDF 载体提取载荷，兼容 V1 旧格式。
func ExtractPDF(carrierPath string) ([]byte, error) {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	eofIdx := bytes.LastIndex(data, []byte("%%EOF"))
	if eofIdx < 0 {
		return nil, errors.New("不是有效的 PDF 文件")
	}

	// 新格式：%%EOF 之前插入
	if idx := bytes.LastIndex(data[:eofIdx], pdfTagV2); idx >= 0 {
		lenStart := idx - 8
		if lenStart < 0 {
			return nil, errors.New("PDF 载荷头部损坏")
		}
		payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
		payloadStart := lenStart - payloadLen
		if payloadStart < 0 {
			return nil, errors.New("PDF 载荷长度非法")
		}
		out := make([]byte, payloadLen)
		copy(out, data[payloadStart:lenStart])
		return out, nil
	}

	// V1 兼容：文件末尾 EOF 追加
	idx := len(data) - len(pdfTagV1)
	if idx >= 0 && bytes.Equal(data[idx:], pdfTagV1) {
		lenStart := idx - 8
		if lenStart < 0 {
			return nil, errors.New("PDF 载荷头部损坏")
		}
		payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
		payloadStart := lenStart - payloadLen
		if payloadStart < 0 {
			return nil, errors.New("PDF 载荷长度非法")
		}
		out := make([]byte, payloadLen)
		copy(out, data[payloadStart:lenStart])
		return out, nil
	}
	return nil, errors.New("载体中未找到 StegGo PDF 载荷")
}

// IsPDF 简单 PDF 头校验。
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF-"))
}
