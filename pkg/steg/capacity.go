package steg

import (
	"fmt"

	"steggo/pkg/carrier"
)

// CapacityResult 载体容量预检测结果。
type CapacityResult struct {
	Kind     string `json:"kind"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Channels int    `json:"channels"`
	BitDepth int    `json:"bit_depth"`
	MaxBits  int64  `json:"max_bits"`
	MaxBytes int64  `json:"max_bytes"`
	Overhead int64  `json:"overhead"`
	Usable   int64  `json:"usable"`
}

// CheckImageCapacity 检测无损图片载体在指定嵌入位数下的容量。
func CheckImageCapacity(path string, bitDepth int) (*CapacityResult, error) {
	img, err := carrier.LoadImage(path)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if bitDepth < 1 || bitDepth > 4 {
		return nil, fmt.Errorf("嵌入位数必须在 1-4 之间")
	}
	maxBits := int64(w) * int64(h) * 3 * int64(bitDepth)
	maxBytes := maxBits / 8
	// 头部开销：magic+固定头+变长名（此处按 0 名估算，实际取决于文件名）
	overhead := int64(77)
	usable := maxBytes - overhead
	if usable < 0 {
		usable = 0
	}
	return &CapacityResult{
		Kind:     "image",
		Width:    w,
		Height:   h,
		Channels: 3,
		BitDepth: bitDepth,
		MaxBits:  maxBits,
		MaxBytes: maxBytes,
		Overhead: overhead,
		Usable:   usable,
	}, nil
}

// CapacityMatrix 返回 1-4 位深度的容量矩阵。
func CapacityMatrix(path string) ([]*CapacityResult, error) {
	var out []*CapacityResult
	for d := 1; d <= 4; d++ {
		r, err := CheckImageCapacity(path, d)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// CheckGenericCapacity 检测通用载体（音频/PDF/视频）容量：按文件大小估算。
func CheckGenericCapacity(path string) (int64, error) {
	info, err := carrier.FileSize(path)
	if err != nil {
		return 0, err
	}
	return info, nil
}
