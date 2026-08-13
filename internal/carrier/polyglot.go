package carrier

import (
	"bytes"
	"fmt"
	"os"
)

// PolyglotFormat 可被 Polyglot 检测的格式标识。
type PolyglotFormat string

const (
	FmtPNG  PolyglotFormat = "png"
	FmtGIF  PolyglotFormat = "gif"
	FmtPDF  PolyglotFormat = "pdf"
	FmtZIP  PolyglotFormat = "zip"
	FmtJPEG PolyglotFormat = "jpeg"
	FmtWAV  PolyglotFormat = "wav"
	FmtMP4  PolyglotFormat = "mp4"
)

// polyglotMagic 各格式魔数表（检测用）。
var polyglotMagic = []struct {
	name  PolyglotFormat
	magic []byte
}{
	{FmtPNG, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}},
	{FmtGIF, []byte("GIF8")},
	{FmtPDF, []byte("%PDF-")},
	{FmtZIP, []byte{'P', 'K', 0x03, 0x04}},
	{FmtJPEG, []byte{0xFF, 0xD8, 0xFF}},
	{FmtWAV, []byte("RIFF")},
	{FmtMP4, []byte("ftyp")},
}

// BuildPolyglot 将多个文件按顺序拼接为一个 Polyglot 文件。
// 输出文件同时携带多个格式的魔数，可由不同解析器各自识别。
func BuildPolyglot(files []string, outPath string) error {
	if len(files) < 2 {
		return fmt.Errorf("Polyglot 至少需要两个输入文件")
	}
	var buf []byte
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", f, err)
		}
		buf = append(buf, b...)
	}
	return os.WriteFile(outPath, buf, 0644)
}

// DetectPolyglot 全文件扫描各格式魔数，返回检测到的格式列表。
// 空列表表示未检测到任何已知格式。
func DetectPolyglot(path string) ([]PolyglotFormat, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var found []PolyglotFormat
	for _, fm := range polyglotMagic {
		if bytes.Index(data, fm.magic) >= 0 {
			found = append(found, fm.name)
		}
	}
	return found, nil
}
