package algorithm

import (
	"bytes"
	"testing"
)

// TestDCTRoundtripRealCarrierQ 测试不同 q 值下的真实载体 roundtrip。
func TestDCTRoundtripRealCarrierQ(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	a := NewDCT()
	seed := []byte("test-seed-dct-real")
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)

	for _, q := range []int{4, 6, 8, 12, 16} {
		work := cloneImg(img)
		opt := Options{Quality: q, Seed: seed}
		if err := a.Embed(work, bits, opt); err != nil {
			t.Fatalf("q=%d embed: %v", q, err)
		}
		gotBits, err := a.Extract(work, Options{Quality: q, Seed: seed})
		if err != nil {
			t.Fatalf("q=%d extract: %v", q, err)
		}
		got := BitsToBytes(gotBits[:len(bits)])
		bad := 0
		for i := range secret {
			if got[i] != secret[i] {
				bad++
			}
		}
		t.Logf("q=%2d 错误字节=%d/%d", q, bad, len(secret))
	}
}

// TestDCTYRoundtripError 量化 RGB→Y→RGB→Y 往返误差。
func TestDCTYRoundtripError(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	maxErr := 0
	var maxAt [2]int
	// 单像素往返：RGB→YUV→RGB→YUV，比较两次 Y
	for y := 0; y < h && y < 16; y++ {
		for x := 0; x < w && x < 16; x++ {
			c := img.NRGBAAt(x, y)
			y1 := rgbToYUV(c.R, c.G, c.B)
			r, g, b := yuvToRGB(y1.Y, y1.Cb, y1.Cr)
			y2 := rgbToYUV(r, g, b)
			diff := int(y1.Y) - int(y2.Y)
			if diff < 0 {
				diff = -diff
			}
			if diff > maxErr {
				maxErr = diff
				maxAt = [2]int{x, y}
			}
		}
	}
	t.Logf("单像素 RGB→YUV→RGB→YUV Y 最大往返误差 = %d at %v", maxErr, maxAt)
}
