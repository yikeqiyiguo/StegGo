package algorithm

import (
	"bytes"
	"math"
	"testing"
)

// embedBlockIter2 迭代回读修正，模拟真实 RGB 往返链路。
// 输入：块内 Y 像素、Cb、Cr。输出：满足 bucket 的 Y 像素。
func embedBlockIter2(pix [64]uint8, cb, cr [64]uint8, q float64, bits24 []byte) ([64]uint8, bool) {
	var coef [64]float64
	for i := range coef {
		coef[i] = float64(pix[i])
	}
	dct8(&coef)

	var out [64]uint8
	for iter := 0; iter < 20; iter++ {
		target := coef
		for n, k := range midFreqIdx {
			target[k] = qimEmbed(coef[k], q, bits24[n])
		}
		// 模拟真实链路：IDCT → round/clamp → RGB 往返 → 提取端 Y
		sim := target
		idct8(&sim)
		var simY [64]uint8
		for i := range simY {
			y := clampByte(int(math.Round(sim[i])))
			simY[i] = roundTripY(y, cb[i], cr[i])
		}
		// 重新 DCT 验证
		var v [64]float64
		for i := range v {
			v[i] = float64(simY[i])
		}
		dct8(&v)
		ok := true
		for n, k := range midFreqIdx {
			if qimExtract(v[k], q) != bits24[n] {
				ok = false
				sign := 1.0
				if v[k] < 0 {
					sign = -1
				}
				abs := math.Abs(v[k])
				tgt := int(math.Floor(abs / q))
				if tgt&1 != int(bits24[n])&1 {
					tgt++
				}
				coef[k] = sign * (float64(tgt)*q + q/2)
			}
		}
		if ok {
			idct8(&target)
			for i := range out {
				out[i] = clampByte(int(math.Round(target[i])))
			}
			return out, true
		}
	}
	return out, false
}

// TestDCTIterRGB 用模拟 RGB 往返的迭代方案测试真实载体。
func TestDCTIterRGB(t *testing.T) {
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
		blocks := 0
		failed := 0
		for by := 0; by < h; by += 8 {
			for bx := 0; bx < w; bx += 8 {
				if bi >= len(bits) {
					break
				}
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
				var bits24 [24]byte
				for n := range bits24 {
					if bi < len(bits) {
						bits24[n] = bits[bi]
					}
					bi++
				}
				blocks++
				out, conv := embedBlockIter2(pix, cb, cr, float64(q), bits24[:])
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
	}
}
