package algorithm

import (
	"bytes"
	"testing"
)

// TestDCTClampSource 定位错误位所在块。
func TestDCTClampSource(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)

	for _, q := range []int{8, 16} {
		work := cloneImg(img)
		if err := NewDCT().Embed(work, bits, Options{Quality: q, Seed: []byte("d")}); err != nil {
			t.Fatal(err)
		}
		gotBits, _ := NewDCT().Extract(work, Options{Quality: q, Seed: []byte("d")})

		badBlocks := map[int]int{}
		for i := range bits {
			if gotBits[i] != bits[i] {
				badBlocks[i/24]++
			}
		}
		t.Logf("q=%d 错误块数=%d 分布=%v", q, len(badBlocks), badBlocks)
	}
}

// TestDCTClampBoundary 检测嵌入后 IDCT 结果是否越界 [16,239]。
func TestDCTClampBoundary(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	secret := bytes.Repeat([]byte("S"), 200)
	bits := ByteToBits(secret)

	for _, q := range []int{8} {
		work := cloneImg(img)
		if err := NewDCT().Embed(work, bits, Options{Quality: q, Seed: []byte("d")}); err != nil {
			t.Fatal(err)
		}
		ycbcr := toYCbCr(work)
		lo, hi := 0, 0
		for i := range ycbcr {
			y := int(ycbcr[i].Y)
			if y < 16 {
				lo++
			}
			if y > 239 {
				hi++
			}
		}
		t.Logf("q=%2d 压缩域边界: <16 有 %d 像素, >239 有 %d 像素", q, lo, hi)
	}
	_ = bits
}
