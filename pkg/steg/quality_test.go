package steg

import (
	"image"
	"image/color"
	"math"
	"testing"

	"steggo/pkg/carrier"
)

func solidImage(w, h int, r, g, b uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	c := color.NRGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestPSNRIdentical(t *testing.T) {
	a := solidImage(64, 64, 100, 150, 200)
	p, err := PSNR(a, a)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(p, 1) {
		t.Fatalf("相同图像 PSNR 应为 +Inf, 实际 %v", p)
	}
}

func TestPSNRDiffers(t *testing.T) {
	a := solidImage(64, 64, 100, 150, 200)
	b := solidImage(64, 64, 101, 150, 200)
	p, err := PSNR(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(p, 1) {
		t.Fatal("不同图像 PSNR 不应为 Inf")
	}
	if p <= 0 || p > 100 {
		t.Fatalf("PSNR 值异常: %v", p)
	}
}

func TestPSNRDimensionMismatch(t *testing.T) {
	a := solidImage(32, 32, 1, 1, 1)
	b := solidImage(33, 32, 1, 1, 1)
	if _, err := PSNR(a, b); err == nil {
		t.Fatal("尺寸不一致应报错")
	}
}

func TestSSIMIdentical(t *testing.T) {
	a := newTestImage(48, 48, 5)
	s := SSIM(a, a)
	if s != 1 {
		t.Fatalf("相同图像 SSIM 应为 1, 实际 %v", s)
	}
}

func TestSSIMNearIdentical(t *testing.T) {
	// 轻微 LSB 扰动 → SSIM 应非常接近 1
	orig := newTestImage(64, 64, 9)
	steg := image.NewNRGBA(orig.Bounds())
	copy(steg.Pix, orig.Pix)
	steg.Pix[0] ^= 0x01 // 修改一个 LSB
	s := SSIM(orig, steg)
	if s < 0.99 {
		t.Fatalf("轻微扰动 SSIM 应 >0.99, 实际 %v", s)
	}
}

func TestSSIMVeryDifferent(t *testing.T) {
	a := solidImage(32, 32, 0, 0, 0)
	b := solidImage(32, 32, 255, 255, 255)
	s := SSIM(a, b)
	if s > 0.5 {
		t.Fatalf("黑白图像 SSIM 应较低, 实际 %v", s)
	}
}

func TestEvaluateQualityReport(t *testing.T) {
	orig := newTestImage(64, 64, 11)
	dir := t.TempDir()
	origPath := dir + "/orig.png"
	stegPath := dir + "/steg.png"
	if err := carrier.SaveImage(orig, origPath); err != nil {
		t.Fatal(err)
	}

	steg := image.NewNRGBA(orig.Bounds())
	copy(steg.Pix, orig.Pix)
	steg.Pix[100] ^= 0x01
	if err := carrier.SaveImage(steg, stegPath); err != nil {
		t.Fatal(err)
	}

	r, err := EvaluateQuality(origPath, stegPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Notes) == 0 {
		t.Fatal("质量报告应包含说明")
	}
	if r.PSNR < 30 {
		t.Fatalf("轻微扰动 PSNR 应较高, 实际 %v", r.PSNR)
	}
}
