package algorithm

import (
	"testing"
)

// TestDCTBlock37Trace 深挖块 37（错误块）的 Y 值链路。
func TestDCTBlock37Trace(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	ycbcr0 := toYCbCr(img)
	// 块 37：by=0, bx=37
	bx := 37 * 8
	by := 0
	t.Logf("图片 %dx%d", w, h)

	var y0 [64]uint8
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			y0[j*8+i] = ycbcr0[(by+j)*w+bx+i].Y
		}
	}
	t.Logf("块37 原始 Y: %v", y0[:16])

	// 模拟 roundTrip：原始 Y → compress → 嵌入 → RGB 往返
	var compressed [64]uint8
	for i := range compressed {
		compressed[i] = uint8(compressY(y0[i]))
	}
	t.Logf("块37 压缩 Y: %v", compressed[:16])

	// 用真实链路：fromYCbCr 写回 → toYCbCr 读回
	work := cloneImg(img)
	ycbcr := toYCbCr(work)
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			ycbcr[(by+j)*w+bx+i].Y = compressed[j*8+i]
		}
	}
	fromYCbCr(work, ycbcr)
	ycbcr2 := toYCbCr(work)
	var yRoundTrip [64]uint8
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			yRoundTrip[j*8+i] = ycbcr2[(by+j)*w+bx+i].Y
		}
	}
	t.Logf("块37 RGB往返Y: %v", yRoundTrip[:16])

	// 对比模拟 roundTripY 与真实 toYCbCr
	var ySim [64]uint8
	for i := range ySim {
		p := ycbcr0[(by)*w+bx+i] // 注意：用原始 Cb/Cr
		ySim[i] = roundTripY(compressed[i], p.Cb, p.Cr)
	}
	t.Logf("块37 模拟Y: %v", ySim[:16])
	diff := 0
	maxDiff := 0
	for i := 0; i < 64; i++ {
		d := int(ySim[i]) - int(yRoundTrip[i])
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
		if ySim[i] != yRoundTrip[i] {
			diff++
		}
	}
	t.Logf("模拟 vs 真实: 差异像素=%d 最大差=%d", diff, maxDiff)
}
