package algorithm

import (
	"bytes"
	"math"
	"testing"
)

// TestDCTPixelDrift 量化"嵌入→IDCT→round/clamp→重新DCT"后 bucket 翻转率。
func TestDCTPixelDrift(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	secret := bytes.Repeat([]byte("S"), 60)
	bits := ByteToBits(secret)

	for _, q := range []int{8, 12, 16, 24, 32} {
		work := cloneImg(img)
		opt := Options{Quality: q, Seed: []byte("diag")}
		if err := NewDCT().Embed(work, bits, opt); err != nil {
			t.Fatal(err)
		}
		ycbcr := toYCbCr(work)
		flips := 0
		total := 0
		bi := 0
		// 只验证前 60 字节对应的块
		needBlocks := (len(bits) + 23) / 24
		for by := 0; by < h && bi/24 < needBlocks; by += 8 {
			for bx := 0; bx < w && bi/24 < needBlocks; bx += 8 {
				block := [64]float64{}
				for j := 0; j < 8; j++ {
					for i := 0; i < 8; i++ {
						x, y := bx+i, by+j
						if x < w && y < h {
							block[j*8+i] = float64(ycbcr[y*w+x].Y)
						} else {
							block[j*8+i] = 128
						}
					}
				}
				dct8(&block)
				for _, k := range midFreqIdx {
					want := byte(0)
					if bi < len(bits) {
						want = bits[bi]
					}
					bi++
					total++
					if qimExtract(block[k], float64(q)) != want {
						flips++
					}
				}
			}
		}
		t.Logf("q=%2d bucket翻转=%d/%d (%.2f%%)", q, flips, total, 100*float64(flips)/float64(total))
	}
}

// TestDCTDriftAfterRound 模拟嵌入系数后立即 IDCT→round→DCT 的漂移幅度。
func TestDCTDriftAfterRound(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	ycbcr := toYCbCr(img)
	maxDrift := 0.0
	for by := 0; by < h; by += 8 {
		for bx := 0; bx < w; bx += 8 {
			block := [64]float64{}
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					if x < w && y < h {
						block[j*8+i] = float64(ycbcr[y*w+x].Y)
					} else {
						block[j*8+i] = 128
					}
				}
			}
			dct8(&block)
			// 中心化一个典型系数
			block[3] = 1*8 + 4 // bucket 中心
			idct8(&block)
			for i := 0; i < 64; i++ {
				block[i] = math.Round(block[i])
			}
			dct8(&block)
			drift := math.Abs(block[3] - 12)
			if drift > maxDrift {
				maxDrift = drift
			}
		}
	}
	t.Logf("单系数中心化后经 IDCT→round→DCT 最大漂移 = %.3f (q=8 容差 4.0)", maxDrift)
}
