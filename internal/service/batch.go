package service

import (
	"fmt"
	"path/filepath"

	"steggo/internal/common"
)

// BatchOptions 批量处理配置。
type BatchOptions struct {
	Options
	// InputDir 批量输入目录（载体文件所在目录）。
	InputDir string
	// OutputDir 批量输出目录。
	OutputDir string
	// Exts 扩展名过滤（默认全部受支持格式）。
	Exts []string
}

// BatchEmbed 将同一秘密文件批量嵌入目录下的每个载体。
// 输出文件写入 OutputDir 下同名文件；跳过超出容量的载体并记录错误。
func (s *Service) BatchEmbed(opt BatchOptions) ([]*Result, error) {
	if opt.InputDir == "" {
		return nil, fmt.Errorf("批量嵌入需要输入目录")
	}
	if opt.OutputDir == "" {
		return nil, fmt.Errorf("批量嵌入需要输出目录")
	}
	if err := common.EnsureDir(opt.OutputDir); err != nil {
		return nil, err
	}
	exts := opt.Exts
	if len(exts) == 0 {
		exts = []string{".png", ".bmp", ".tif", ".tiff", ".wav", ".flac", ".pdf", ".mp4", ".mkv", ".txt", ".md"}
	}
	var files []string
	if err := common.WalkFilesByExt(opt.InputDir, exts, &files); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("输入目录中没有匹配的载体文件")
	}

	var results []*Result
	for _, f := range files {
		o := opt.Options
		o.CarrierPath = f
		o.OutputPath = filepath.Join(opt.OutputDir, filepath.Base(f))
		res, err := s.Embed(o)
		if err != nil {
			results = append(results, &Result{Name: filepath.Base(f), Size: -1})
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// BatchExtract 批量提取目录下所有载体中的秘密数据。
// 每个载体输出到 OutputDir/<载体名>/ 子目录；失败的载体记录错误。
func (s *Service) BatchExtract(opt BatchOptions) ([]*Result, error) {
	if opt.InputDir == "" {
		return nil, fmt.Errorf("批量提取需要输入目录")
	}
	if opt.OutputDir == "" {
		return nil, fmt.Errorf("批量提取需要输出目录")
	}
	if err := common.EnsureDir(opt.OutputDir); err != nil {
		return nil, err
	}
	exts := opt.Exts
	if len(exts) == 0 {
		exts = []string{".png", ".bmp", ".tif", ".tiff", ".wav", ".flac", ".pdf", ".mp4", ".mkv", ".txt", ".md"}
	}
	var files []string
	if err := common.WalkFilesByExt(opt.InputDir, exts, &files); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("输入目录中没有匹配的载体文件")
	}

	var results []*Result
	for _, f := range files {
		o := opt.Options
		o.CarrierPath = f
		base := filepath.Base(f)
		o.OutputPath = filepath.Join(opt.OutputDir, baseWithoutExt(base))
		res, err := s.Extract(o)
		if err != nil {
			results = append(results, &Result{Name: base, Size: -1})
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// batchSummary 统计批量结果。
func batchSummary(results []*Result) (ok, fail int) {
	for _, r := range results {
		if r.Size >= 0 {
			ok++
		} else {
			fail++
		}
	}
	return ok, fail
}

// baseWithoutExt 去除扩展名的文件名。
func baseWithoutExt(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}
