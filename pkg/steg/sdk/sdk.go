// Package sdk 提供 StegGo 的公开统一 SDK（V2.0 能力面）。
//
// 背景：cmd/gui 是独立 Go module（replace steggo => ../..），
// 受 internal 规则限制只能导入 steggo/pkg/...，无法直接使用
// steggo/internal/service（V2 业务层：七算法嵌入/自动扫描提取/
// 水印/批量/Shamir 分权/容量/质量/自检审计）。
//
// 本包作为 pkg/steg 门面之上的 V2 封装，把 internal/service、
// internal/algorithm、internal/carrier 的全部能力以公开 API 暴露，
// 使 GUI 与 CLI/TUI 功能完全对齐。
//
// 依赖图：pkg/steg/sdk → internal/service → pkg/steg（V1 兼容），无环。
package sdk

import (
	"steggo/internal/algorithm"
	"steggo/internal/service"
)

// Options 一次嵌入/提取的统一配置（与 internal/service.Options 完全一致）。
type Options = service.Options

// Result 一次操作的结果摘要。
type Result = service.Result

// BatchOptions 批量处理配置。
type BatchOptions = service.BatchOptions

// ShamirOptions 分权（Shamir 密钥分享）配置。
type ShamirOptions = service.ShamirOptions

// Algorithms 返回全部已注册算法名称（稳定顺序），供 UI 下拉选择。
func Algorithms() []string { return algorithm.Names() }

// Embed 将秘密数据加密后嵌入载体，输出到 opt.OutputPath。
func Embed(opt Options) (*Result, error) {
	return service.New().Embed(opt)
}

// Extract 从载体提取并解密秘密数据，写出到 opt.OutputPath 目录。
// 自动扫描算法参数组合（V2.0），失败后回退 V1.0 兼容路径。
func Extract(opt Options) (*Result, error) {
	return service.New().Extract(opt)
}

// EmbedWatermark 将水印标记嵌入图像（LSB depth=1，固定种子）。
func EmbedWatermark(imgPath, outPath, mark string) (*Result, error) {
	return service.New().EmbedWatermark(imgPath, outPath, mark)
}

// ExtractWatermark 从图像提取水印标记（自动扫描 LSB 深度 1-4）。
func ExtractWatermark(imgPath string) (string, error) {
	return service.New().ExtractWatermark(imgPath)
}

// BatchEmbed 将同一秘密文件批量嵌入目录下的每个载体。
func BatchEmbed(opt BatchOptions) ([]*Result, error) {
	return service.New().BatchEmbed(opt)
}

// BatchExtract 批量提取目录下所有载体中的秘密数据。
func BatchExtract(opt BatchOptions) ([]*Result, error) {
	return service.New().BatchExtract(opt)
}

// SplitToCarriers 将秘密数据加密后按 Shamir 分片，依次嵌入 Total 个载体。
func SplitToCarriers(opt ShamirOptions) ([]*Result, error) {
	return service.New().SplitToCarriers(opt)
}

// RecoverFromCarriers 从分片载体中恢复秘密：提取分片 → Shamir 恢复 → 解密 → 写出。
func RecoverFromCarriers(opt ShamirOptions) (*Result, error) {
	return service.New().RecoverFromCarriers(opt)
}
