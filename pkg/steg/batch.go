package steg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"steggo/pkg/carrier"
)

// BatchResult 单文件处理结果。
type BatchResult struct {
	Carrier string  `json:"carrier"`
	Output  string  `json:"output,omitempty"`
	Error   error   `json:"-"`
	Result  *Result `json:"result,omitempty"`
}

// BatchEmbedOptions 批量嵌入配置。
type BatchEmbedOptions struct {
	Options
	// Concurrency 并发数，默认 4。
	Concurrency int
	// IncludeDirs 是否递归子目录。
	IncludeDirs bool
}

// BatchExtractOptions 批量提取配置。
type BatchExtractOptions struct {
	Concurrency int
	IncludeDirs bool
}

// BatchEmbed 将同一秘密批量嵌入载体目录中的全部支持载体。
func BatchEmbed(ctx context.Context, carrierDir, outputDir, secretPath string, opts BatchEmbedOptions) ([]BatchResult, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	carriers, err := collectCarriers(carrierDir, opts.IncludeDirs)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	results := make([]BatchResult, 0, len(carriers))
	var mu sync.Mutex
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	for _, c := range carriers {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			rel, _ := filepath.Rel(carrierDir, c)
			out := filepath.Join(outputDir, rel)
			if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				mu.Lock()
				results = append(results, BatchResult{Carrier: c, Error: err})
				mu.Unlock()
				return
			}
			res, err := AutoEmbed(c, out, secretPath, opts.Options)
			mu.Lock()
			results = append(results, BatchResult{Carrier: c, Output: out, Error: err, Result: res})
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, nil
}

// BatchExtract 批量提取载体目录中全部载体的秘密。
func BatchExtract(ctx context.Context, carrierDir, outputDir string, password []byte, opts BatchExtractOptions) ([]BatchResult, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	carriers, err := collectCarriers(carrierDir, opts.IncludeDirs)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	results := make([]BatchResult, 0, len(carriers))
	var mu sync.Mutex
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	for _, c := range carriers {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			res, err := AutoExtract(c, outputDir, password)
			mu.Lock()
			results = append(results, BatchResult{Carrier: c, Error: err, Result: res})
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, nil
}

// collectCarriers 收集目录下所有受支持的载体文件。
func collectCarriers(dir string, recursive bool) ([]string, error) {
	var out []string
	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if carrier.IsSupported(path) {
			out = append(out, path)
		}
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, err
	}
	return out, nil
}

// CountSupported 统计目录下受支持载体数量。
func CountSupported(dir string, recursive bool) (int, error) {
	list, err := collectCarriers(dir, recursive)
	return len(list), err
}

// FilterSupportedExts 返回支持列表（诊断用）。
func FilterSupportedExts(exts []string) []string {
	var out []string
	for _, e := range exts {
		if strings.HasPrefix(e, ".") {
			if _, err := carrier.DetectKind("x" + e); err == nil {
				out = append(out, e)
			}
		}
	}
	return out
}
