package algorithm

import (
	"bytes"
	"testing"
)

// TestDCTPOCS 测试 POCS 迭代方案。
func TestDCTPOCS(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)

	for _, q := range []int{8, 16} {
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
		blocks, failed := 0, 0
		// 只嵌入完整 8×8 块：含越界像素的块不参与，保证 POCS 收敛（见 TestDCTDiagBlockTrace）
		for by := 0; by+8 <= h; by += 8 {
			for bx := 0; bx+8 <= w; bx += 8 {
				if bi >= len(bits) {
					break
				}
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
				blocks++
				out, conv := embedBlockPOCS(pix, cb, cr, mask, float64(q), bits24[:])
				if !conv {
					failed++
					continue
				}
				for j := 0; j < 8; j++ {
					for i := 0; i < 8; i++ {
						x, y := bx+i, by+j
						if x < w && y < h {
							compressed[y*w+x] = out[j*8+i]
						}
					}
				}
			}
		}
		for i := range compressed {
			ycbcr[i].Y = compressed[i]
		}
		fromYCbCr(work, ycbcr)
		gotBits, _ := NewDCT().Extract(work, Options{Quality: q, Seed: []byte("d")})
		got := BitsToBytes(gotBits[:len(bits)])
		bad := 0
		for i := range secret {
			if got[i] != secret[i] {
				bad++
			}
		}
		t.Logf("q=%2d 块=%d 未收敛=%d 错误字节=%d/%d", q, blocks, failed, bad, len(secret))

		// 定位错误块，反向验证 POCS 嵌入像素的 bucket
		badBlocks := map[int]int{}
		for i := range bits {
			if gotBits[i] != bits[i] {
				badBlocks[i/24]++
			}
		}
		fullCols := w / 8 // 完整块列数
		for bIdx := range badBlocks {
			bx := (bIdx % fullCols) * 8
			by := (bIdx / fullCols) * 8
			var pix, cb, cr [64]uint8
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					if x < w && y < h {
						pix[j*8+i] = compressed[y*w+x]
						cb[j*8+i] = cbs[y*w+x]
						cr[j*8+i] = crs[y*w+x]
					} else {
						pix[j*8+i] = 128
						cb[j*8+i] = 128
						cr[j*8+i] = 128
					}
				}
			}
			// 用写回的像素重新验证
			var v [64]float64
			for i := range v {
				v[i] = float64(roundTripY(pix[i], cb[i], cr[i]))
			}
			dct8(&v)
			nBad := 0
			for n := 0; n < 24; n++ {
				if qimExtract(v[midFreqIdx[n]], float64(q)) != bits24At(bits, bIdx*24+n) {
					nBad++
				}
			}
			t.Logf("  错误块%d(%d,%d): 写回像素重验错误=%d", bIdx, bx/8, by/8, nBad)
		}
	}
}

func bits24At(bits []byte, pos int) byte {
	if pos < len(bits) {
		return bits[pos]
	}
	return 0
}
