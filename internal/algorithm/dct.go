package algorithm

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
)

// dct 8×8 块 DCT 域 QIM 隐写。
//
// 流程：RGB → YCbCr（BT.601）→ Y 压缩到 [16,239]（防 IDCT 越界 clamp）→
// 8×8 分块 DCT-II → 在中频系数上做奇偶量化嵌入
// （bucket = floor(|k|/q)，bucket 奇偶性编码 bit）→ IDCT → 转回 RGB。
//
// 量化步长 q 由 Options.Quality 控制：q 越大对噪声/舍入越鲁棒，但失真越大。
// Y 压缩与 DWT 一致：嵌入后像素保持在 [16,239]，IDCT 舍入不会越界被 clamp
// 截断，保证系数往返可逆。提取无需原图（盲提取）：重新 DCT 后读 bucket 奇偶。
type dct struct{}

// NewDCT 创建 DCT 算法实例。
func NewDCT() Algorithm { return &dct{} }

func (a *dct) ID() byte     { return IDDCT }
func (a *dct) Name() string { return "dct" }

// coefPerBlock 每块嵌入的系数个数（zigzag 序 3..26，跳过 DC 与最高频）。
const coefPerBlock = 24

// zigzag 表：zigzag[i] = 8×8 块内按之字形扫描第 i 个位置的线性索引。
var zigzag = func() [64]int {
	tbl := [64]int{}
	// 之字形扫描方向：向右上 (+1,-1)，撞边转向
	r, c, dir := 0, 0, 1 // dir=1 右上, -1 左下
	for i := 0; i < 64; i++ {
		tbl[i] = r*8 + c
		if dir == 1 { // 右上
			if c == 7 {
				r++
				dir = -1
			} else if r == 0 {
				c++
				dir = -1
			} else {
				r--
				c++
			}
		} else { // 左下
			if r == 7 {
				c++
				dir = 1
			} else if c == 0 {
				r++
				dir = 1
			} else {
				r++
				c--
			}
		}
	}
	return tbl
}()

// midFreqIdx 每块嵌入的系数线性索引（zigzag 序 3..26 共 24 个中频系数，跳过 DC 与最高频）。
var midFreqIdx = func() []int {
	z := zigzag
	idx := make([]int, coefPerBlock)
	for i := 0; i < coefPerBlock; i++ {
		idx[i] = z[3+i]
	}
	return idx
}()

// Capacity 返回最大可嵌入位数。
// 只统计完整 8×8 块（宽/高整除 8）：含越界像素的边缘块不参与嵌入，
// 否则"越界像素固定填充"与"嵌入中频系数"两个约束冲突会导致系数往返不可逆。
func (a *dct) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	if err := opt.fillDefaults(); err != nil {
		return 0
	}
	b := img.Bounds()
	nx := b.Dx() / 8
	ny := b.Dy() / 8
	return nx * ny * coefPerBlock
}

// Embed 将位流嵌入 DCT 中频系数。
//
// 采用 POCS 迭代：每块从整数像素出发，DCT→嵌入→IDCT→round，
// 再经 RGB 往返（模拟提取端真实读取链路）验证 bucket 是否满足目标位，
// 不满足则以新像素为起点继续投影，直到收敛。保证"写入像素→提取读取"
// 整条链路可逆，杜绝嵌入后无法提取。
func (a *dct) Embed(img *image.NRGBA, bits []byte, opt Options) error {
	if img == nil {
		return errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return err
	}
	cap := a.Capacity(img, opt)
	if len(bits) > cap {
		return errCapacity(cap, len(bits))
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// 1. 转 YCbCr，并将 Y 压缩到 [16,239]（为 DCT 系数修改预留边界，防 clamp 截断）
	ycbcr := toYCbCr(img)
	compressed := make([]uint8, w*h)
	cbs := make([]uint8, w*h)
	crs := make([]uint8, w*h)
	for i := range compressed {
		compressed[i] = uint8(compressY(ycbcr[i].Y))
		cbs[i] = ycbcr[i].Cb
		crs[i] = ycbcr[i].Cr
	}

	// 2. 完整 8×8 块 POCS 嵌入。
	// 只嵌入含真实数据的块（bi < len(bits)），数据之后的块保持原始像素不动：
	// 提取端读这些块得到的位流与嵌入端一致（均源自原始像素），且头部按长度解析、噪声位不影响数据。
	bi := 0
	q := float64(opt.Quality)
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
			// 每块 24 位；末尾不足 24 位的用 0 补齐（POCS 验证过的收敛模式）
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
			out, ok := embedBlockPOCS(pix, cb, cr, mask, q, bits24[:])
			if !ok {
				return fmt.Errorf("DCT 嵌入块(bx=%d,by=%d) POCS 未收敛，请将 Quality 调至 4~16 范围内（当前 %d）", bx, by, opt.Quality)
			}
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					compressed[(by+j)*w+bx+i] = out[j*8+i]
				}
			}
		}
	}

	// 2.5 将压缩域 Y 写回 ycbcr，再转 RGB
	for i := range compressed {
		ycbcr[i].Y = compressed[i]
	}

	// 3. 转回 RGB
	fromYCbCr(img, ycbcr)
	return nil
}

// Extract 从 DCT 中频系数读取位流。
func (a *dct) Extract(img *image.NRGBA, opt Options) ([]byte, error) {
	if img == nil {
		return nil, errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ycbcr := toYCbCr(img)
	// 嵌入后的图像 Y 已处于压缩域 [16,239]，直接读取（不再二次压缩），
	// 与 DWT 提取端一致，保证系数往返可逆。
	q := float64(opt.Quality)

	bits := make([]byte, 0, a.Capacity(img, opt))
	for by := 0; by+8 <= h; by += 8 {
		for bx := 0; bx+8 <= w; bx += 8 {
			block := [64]float64{}
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					block[j*8+i] = float64(ycbcr[y*w+x].Y)
				}
			}
			dct8(&block)
			for _, k := range midFreqIdx {
				bits = append(bits, qimExtract(block[k], q))
			}
		}
	}
	return bits, nil
}

// roundTripY 模拟嵌入后 Y 经 RGB 往返（fromYCbCr→toYCbCr）读回的实际值，
// 与提取端真实读取链路完全一致。
func roundTripY(y, cb, cr uint8) uint8 {
	r, g, b := yuvToRGB(y, cb, cr)
	return rgbToYUV(r, g, b).Y
}

// embedBlockPOCS POCS 迭代嵌入单个 8×8 块。
// 每轮从整数像素出发，DCT→嵌入→IDCT→round，再经 RGB 往返验证 bucket，
// 不满足则以新像素为起点继续投影。mask[i]==false 表示越界像素固定填充
// （完整块下恒为 true），与提取端构造一致。
//
// 采用多起点：先原始像素，再若干确定性小扰动。平坦+高饱和区域中
// 目标 bucket 与 RGB 往返漂移冲突时，POCS 可能陷入不动点陷阱（不动点不存在），
// 扰动起点可跳出该区域。
func embedBlockPOCS(pix, cb, cr [64]uint8, mask [64]bool, q float64, bits24 []byte) ([64]uint8, bool) {
	starts := [][64]uint8{pix}
	rng := NewRNG([]byte{0xDC, 0x1B, byte(len(bits24)), byte(q)})
	for r := 0; r < 6; r++ {
		var s [64]uint8
		for i := range s {
			d := 0
			for b := 0; b < 4; b++ {
				d += int(rng.NextBit())
			}
			v := int(pix[i]) + d - 2
			if v < 0 {
				v = 0
			}
			if v > 255 {
				v = 255
			}
			s[i] = uint8(v)
		}
		starts = append(starts, s)
	}
	for _, start := range starts {
		if out, ok := pocsRun(start, cb, cr, mask, q, bits24); ok {
			return out, true
		}
	}
	return pix, false
}

// pocsRun 从给定起点运行 POCS 投影迭代，返回满足验证的像素。
func pocsRun(start, cb, cr [64]uint8, mask [64]bool, q float64, bits24 []byte) ([64]uint8, bool) {
	cur := start
	for iter := 0; iter < 40; iter++ {
		var coef [64]float64
		for i := range coef {
			coef[i] = float64(cur[i])
		}
		dct8(&coef)
		for n, k := range midFreqIdx {
			coef[k] = qimEmbed(coef[k], q, bits24[n])
		}
		idct8(&coef)
		var next [64]uint8
		for i := range next {
			if !mask[i] {
				next[i] = 128 // 越界像素固定，禁止修改
				continue
			}
			next[i] = clampByte(int(math.Round(coef[i])))
		}
		// 验证：与提取端完全对齐——有效像素走 RGB 往返，越界像素固定 128
		var v [64]float64
		for i := range v {
			if !mask[i] {
				v[i] = 128
				continue
			}
			v[i] = float64(roundTripY(next[i], cb[i], cr[i]))
		}
		dct8(&v)
		ok := true
		for n, k := range midFreqIdx {
			if qimExtract(v[k], q) != bits24[n] {
				ok = false
				break
			}
		}
		if ok {
			return next, true
		}
		cur = next
	}
	return cur, false
}

// qimEmbed 将 bit 嵌入系数：把系数推至目标 bucket 的中心（容差 q/2），
// 抵抗 YCbCr 舍入与浮点误差。bucket = floor(|k|/q)，奇偶性编码 bit。
func qimEmbed(k, q float64, bit byte) float64 {
	if q < 1 {
		q = 1
	}
	sign := 1.0
	if k < 0 {
		sign = -1
	}
	abs := math.Abs(k)
	target := int(math.Floor(abs / q))
	if target&1 != int(bit)&1 {
		if target > 0 {
			target--
		} else {
			target++
		}
	}
	// 中心化：系数落在 bucket 中点，提取容差 = q/2
	return sign * (float64(target)*q + q/2)
}

// qimExtract 从系数读取 bit。
func qimExtract(k, q float64) byte {
	if q < 1 {
		q = 1
	}
	bucket := math.Floor(math.Abs(k) / q)
	return byte(int(bucket) & 1)
}

// yuvPixel YCbCr 像素。
type yuvPixel struct{ Y, Cb, Cr uint8 }

// toYCbCr 将 NRGBA 转 YCbCr（BT.601 整数近似）。
func toYCbCr(img *image.NRGBA) []yuvPixel {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]yuvPixel, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x+b.Min.X, y+b.Min.Y)
			out[y*w+x] = rgbToYUV(c.R, c.G, c.B)
		}
	}
	return out
}

// fromYCbCr 将 YCbCr 写回 NRGBA。
func fromYCbCr(img *image.NRGBA, ycbcr []yuvPixel) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := ycbcr[y*w+x]
			r, g, bl := yuvToRGB(p.Y, p.Cb, p.Cr)
			img.SetNRGBA(x+b.Min.X, y+b.Min.Y, color.NRGBA{R: r, G: g, B: bl, A: 255})
		}
	}
}

// rgbToYUV BT.601 全范围转换。
func rgbToYUV(r, g, b uint8) yuvPixel {
	Y := (77*int(r) + 150*int(g) + 29*int(b)) >> 8
	Cb := ((-43*int(r) - 85*int(g) + 128*int(b)) >> 8) + 128
	Cr := ((128*int(r) - 107*int(g) - 21*int(b)) >> 8) + 128
	return yuvPixel{clampByte(Y), clampByte(Cb), clampByte(Cr)}
}

// yuvToRGB BT.601 全范围还原。
func yuvToRGB(y, cb, cr uint8) (r, g, b uint8) {
	Y := int(y)
	CB := int(cb) - 128
	CR := int(cr) - 128
	R := (256*Y + 359*CR) >> 8
	G := (256*Y - 88*CB - 183*CR) >> 8
	B := (256*Y + 454*CB) >> 8
	return clampByte(R), clampByte(G), clampByte(B)
}

// dct8 二维 DCT-II（行列分离）。
func dct8(block *[64]float64) {
	// 行
	var row [8]float64
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			row[i] = block[j*8+i]
		}
		dct1d8(&row)
		for i := 0; i < 8; i++ {
			block[j*8+i] = row[i]
		}
	}
	// 列
	var col [8]float64
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			col[j] = block[j*8+i]
		}
		dct1d8(&col)
		for j := 0; j < 8; j++ {
			block[j*8+i] = col[j]
		}
	}
}

// idct8 二维 IDCT。
func idct8(block *[64]float64) {
	var col [8]float64
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			col[j] = block[j*8+i]
		}
		idct1d8(&col)
		for j := 0; j < 8; j++ {
			block[j*8+i] = col[j]
		}
	}
	var row [8]float64
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			row[i] = block[j*8+i]
		}
		idct1d8(&row)
		for i := 0; i < 8; i++ {
			block[j*8+i] = row[i]
		}
	}
}

// dct1d8 一维 DCT-II（正交归一化）。
func dct1d8(v *[8]float64) {
	var out [8]float64
	for k := 0; k < 8; k++ {
		var sum float64
		for n := 0; n < 8; n++ {
			sum += v[n] * math.Cos(math.Pi*float64(k)*float64(2*n+1)/16)
		}
		c := 1.0
		if k == 0 {
			c = 1 / math.Sqrt2
		}
		out[k] = c * sum / 2
	}
	*v = out
}

// idct1d8 一维 IDCT。
func idct1d8(v *[8]float64) {
	var out [8]float64
	for n := 0; n < 8; n++ {
		var sum float64
		for k := 0; k < 8; k++ {
			c := 1.0
			if k == 0 {
				c = 1 / math.Sqrt2
			}
			sum += c * v[k] * math.Cos(math.Pi*float64(k)*float64(2*n+1)/16)
		}
		out[n] = sum / 2
	}
	*v = out
}

// clampByte 裁剪到 [0,255]。
func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
