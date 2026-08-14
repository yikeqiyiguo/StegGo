package algorithm

import (
	"bytes"
	"fmt"
	"testing"
)

// TestDCTQRangeScan 扫描 q=1..32 下数据块的 POCS 收敛性，确定安全上限。
func TestDCTQRangeScan(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)

	for q := 1; q <= 32; q++ {
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
		bi := 0
		fail := 0
		var failBlocks []int
		for by := 0; by+8 <= h; by += 8 {
			for bx := 0; bx+8 <= w; bx += 8 {
				if bi >= len(bits) {
					break
				}
				var pix, cb, cr [64]uint8
				for j := 0; j < 8; j++ {
					for i := 0; i < 8; i++ {
						x, y := bx+i, by+j
						pix[j*8+i] = compressed[y*w+x]
						cb[j*8+i] = cbs[y*w+x]
						cr[j*8+i] = crs[y*w+x]
					}
				}
				var bits24 [24]byte
				for n := range bits24 {
					if bi < len(bits) {
						bits24[n] = bits[bi]
					}
					bi++
				}
				var mask [64]bool
				for i := range mask {
					mask[i] = true
				}
				_, ok := embedBlockPOCS(pix, cb, cr, mask, float64(q), bits24[:])
				if !ok {
					fail++
					if len(failBlocks) < 6 {
						failBlocks = append(failBlocks, (by/8)*(w/8)+bx/8)
					}
				}
			}
		}
		status := "OK"
		if fail > 0 {
			status = fmt.Sprintf("失败 %d 块 %v", fail, failBlocks)
		}
		t.Logf("q=%2d 数据块未收敛: %s", q, status)
	}
}
