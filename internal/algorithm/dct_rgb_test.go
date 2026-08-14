package algorithm

import (
	"bytes"
	"testing"
)

// TestDCTIterBlockTrace 追踪迭代验证通过但真实提取失败的块。
func TestDCTIterBlockTrace(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	bits := ByteToBits(secret)
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
	bi := 0
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
			out, conv := embedBlockIter2(pix, cb, cr, q, bits24[:])
			if !conv {
				t.Logf("块(%d,%d) 未收敛", bx/8, by/8)
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
	// 真实写回 + 提取
	for i := range compressed {
		ycbcr[i].Y = compressed[i]
	}
	fromYCbCr(work, ycbcr)
	gotBits, _ := NewDCT().Extract(work, Options{Quality: 8, Seed: []byte("d")})

	// 定位错误块
	badBlocks := map[int]int{}
	for i := range bits {
		if gotBits[i] != bits[i] {
			badBlocks[i/24]++
		}
	}
	t.Logf("错误块=%v", badBlocks)

	// 对所有错误块做深入追踪
	seen := 0
	for blockIdx := range badBlocks {
		if seen >= 3 {
			break
		}
		seen++
		by := (blockIdx / (w / 8)) * 8
		bx := (blockIdx % (w / 8)) * 8
		// 追踪该块迭代时的验证 vs 真实
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
		// 真实提取读到的 Y（经 RGB 往返后）
		ycbcrReal := toYCbCr(work)
		var realY [64]uint8
		for j := 0; j < 8; j++ {
			for i := 0; i < 8; i++ {
				x, y := bx+i, by+j
				realY[j*8+i] = ycbcrReal[y*w+x].Y
			}
		}
		// 迭代模拟的 Y
		var simY [64]uint8
		for i := range simY {
			simY[i] = roundTripY(pix[i], cb[i], cr[i])
		}
		// 对比
		diffs := 0
		maxDiff := 0
		for i := 0; i < 64; i++ {
			d := int(realY[i]) - int(simY[i])
			if d < 0 {
				d = -d
			}
			if d > maxDiff {
				maxDiff = d
			}
			if realY[i] != simY[i] {
				diffs++
			}
		}
		// 模拟 Y 的 DCT bucket 与真实 Y 的 DCT bucket
		var sb, rb [64]float64
		for i := range sb {
			sb[i] = float64(simY[i])
			rb[i] = float64(realY[i])
		}
		dct8(&sb)
		dct8(&rb)
		badSim, badReal := 0, 0
		for n := 0; n < 24; n++ {
			k := midFreqIdx[n]
			if qimExtract(sb[k], q) != bits24At(bits, blockIdx*24+n) {
				badSim++
			}
			if qimExtract(rb[k], q) != bits24At(bits, blockIdx*24+n) {
				badReal++
			}
		}
		t.Logf("块%d(%d,%d): 像素差异%d 最大%d | 模拟错误%d 真实错误%d", blockIdx, bx/8, by/8, diffs, maxDiff, badSim, badReal)
	}
}
