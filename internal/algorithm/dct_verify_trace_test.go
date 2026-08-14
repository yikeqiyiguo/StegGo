package algorithm

import (
	"testing"
)

// TestDCTVerifyMismatch 验证完整块下 POCS 验证（模拟）与真实提取链路完全一致。
// 生产代码只嵌入完整 8×8 块（跳过含越界像素的边缘块），
// 因此模拟（roundTripY）必须与真实链路（fromYCbCr→toYCbCr）逐像素相同。
func TestDCTVerifyMismatch(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	q := 8.0

	work := cloneImg(img)
	ycbcr := toYCbCr(work)
	compressed := make([]uint8, w*h)
	cbs := make([]uint8, w*h)
	crs := make([]uint8, w*h)
	for i := range compressed {
		compressed[i] = uint8(compressY(ycbcr[i].Y))
		cbs[i] = ycbcr[i].Cb
		crs[i] = ycbcr[i].Cr
	}

	// 完整块（bx=8, by=0），不含越界像素
	bi := 24
	secret := make([]byte, 700)
	for i := range secret {
		secret[i] = byte(i * 7)
	}
	bits := ByteToBits(secret)

	bx := 8
	by := 0
	var pix, cb, cr [64]uint8
	var mask [64]bool
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			x, y := bx+i, by+j
			if x < w && y < h {
				pix[j*8+i] = compressed[y*w+x]
				cb[j*8+i] = cbs[y*w+x]
				cr[j*8+i] = crs[y*w+x]
				mask[j*8+i] = true
			} else {
				pix[j*8+i] = 128
				cb[j*8+i] = 128
				cr[j*8+i] = 128
			}
		}
	}
	var bits24 [24]byte
	for n := range bits24 {
		if bi < len(bits) {
			bits24[n] = bits[bi]
		}
		bi++
	}
	t.Logf("块1 目标位: %v", bits24)

	out, conv := embedBlockPOCS(pix, cb, cr, mask, q, bits24[:])
	t.Logf("块1 POCS收敛=%v", conv)
	if !conv {
		t.Fatal("POCS 应收敛")
	}
	t.Logf("块1 out Y: %v", out[:16])

	// 重验：out 经 roundTripY 的 DCT bucket
	var v [64]float64
	for i := range v {
		v[i] = float64(roundTripY(out[i], cb[i], cr[i]))
	}
	dct8(&v)
	for n := 0; n < 24; n++ {
		k := midFreqIdx[n]
		got := qimExtract(v[k], q)
		want := bits24[n]
		if got != want {
			t.Errorf("  n=%d k=%d coef=%.3f got=%d want=%d FAIL", n, k, v[k], got, want)
		}
	}

	// 模拟真实链路：写回 → fromYCbCr → toYCbCr
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			x, y := bx+i, by+j
			if x < w && y < h {
				compressed[y*w+x] = out[j*8+i]
			}
		}
	}
	for i := range compressed {
		ycbcr[i].Y = compressed[i]
	}
	fromYCbCr(work, ycbcr)
	ycbcr2 := toYCbCr(work)
	var realY [64]uint8
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			x, y := bx+i, by+j
			realY[j*8+i] = ycbcr2[y*w+x].Y
		}
	}
	var simY [64]uint8
	for i := range simY {
		simY[i] = roundTripY(out[i], cb[i], cr[i])
	}
	t.Logf("块1 模拟Y: %v", simY[:16])
	t.Logf("块1 真实Y: %v", realY[:16])
	diff := 0
	for i := range realY {
		if realY[i] != simY[i] {
			diff++
		}
	}
	t.Logf("块1 模拟vs真实差异像素=%d", diff)
	if diff != 0 {
		t.Fatalf("完整块下模拟应与真实链路一致，差异像素=%d", diff)
	}
}
