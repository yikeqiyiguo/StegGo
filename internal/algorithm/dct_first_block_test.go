package algorithm

import (
	"bytes"
	"testing"
)

// TestDCTFirstBlockDebug 检查第一个 8×8 块的嵌入系数。
func TestDCTFirstBlockDebug(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	a := NewDCT()
	seed := []byte("test-seed-dct-real")
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)
	opt := Options{Quality: 8, Seed: seed}

	// 嵌入前的 Y
	ycbcr0 := toYCbCr(img)
	t.Logf("原图第一块 Y[0..7] = %v", ycbcr0[0:8])

	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatal(err)
	}

	// 嵌入后的 Y
	ycbcr1 := toYCbCr(img)
	t.Logf("嵌入后第一块 Y[0..7] = %v", ycbcr1[0:8])

	// 重新 DCT 第一块，看 bucket
	w := img.Bounds().Dx()
	block := [64]float64{}
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			block[j*8+i] = float64(ycbcr1[j*w+i].Y)
		}
	}
	dct8(&block)
	for n, k := range midFreqIdx {
		bit := bits[n]
		got := qimExtract(block[k], 8)
		t.Logf("k=%d coef=%.2f want=%d got=%d %s", k, block[k], bit, got, map[bool]string{true: "OK", false: "FAIL"}[bit == got])
	}
}
