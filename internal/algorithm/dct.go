package algorithm

import (
	"errors"
	"image"
	"image/color"
	"math"
)

// dct 8×8 块 DCT 域 QIM 隐写。
//
// 流程：RGB → YCbCr（BT.601）→ 8×8 分块 DCT-II → 在中频系数上做
// 奇偶量化嵌入（bucket = floor(|k|/q)，bucket 奇偶性编码 bit）→ IDCT → 转回 RGB。
//
// 量化步长 q 由 Options.Quality 控制：q 越大对噪声/舍入越鲁棒，但失真越大。
// 提取无需原图（盲提取）：重新 DCT 后读 bucket 奇偶。
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
func (a *dct) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	if err := opt.fillDefaults(); err != nil {
		return 0
	}
	b := img.Bounds()
	nx := (b.Dx() + 7) / 8
	ny := (b.Dy() + 7) / 8
	return nx * ny * coefPerBlock
}

// Embed 将位流嵌入 DCT 中频系数。
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

	// 1. 转 YCbCr
	ycbcr := toYCbCr(img)

	// 2. 分块嵌入
	bi := 0
	// 噪声填充 RNG：DCT 系数区不存噪声，剩余 bit 由数据位流末尾噪声补齐（供统计）。
	rng := NewRNG(append(opt.Seed, 0xD7))
	q := opt.Quality
	for by := 0; by < h; by += 8 {
		for bx := 0; bx < w; bx += 8 {
			block := [64]float64{}
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					if x >= w || y >= h {
						block[j*8+i] = 128
					} else {
						block[j*8+i] = float64(ycbcr[y*w+x].Y)
					}
				}
			}
			dct8(&block)
			for _, k := range midFreqIdx {
				bit := byte(0)
				if bi < len(bits) {
					bit = bits[bi]
				} else {
					bit = rng.NextBit()
				}
				bi++
				block[k] = qimEmbed(block[k], float64(q), bit)
			}
			idct8(&block)
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					if x >= w || y >= h {
						continue
					}
					ycbcr[y*w+x].Y = clampByte(int(math.Round(block[j*8+i])))
				}
			}
		}
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
	q := opt.Quality

	bits := make([]byte, 0, a.Capacity(img, opt))
	for by := 0; by < h; by += 8 {
		for bx := 0; bx < w; bx += 8 {
			block := [64]float64{}
			for j := 0; j < 8; j++ {
				for i := 0; i < 8; i++ {
					x, y := bx+i, by+j
					if x >= w || y >= h {
						block[j*8+i] = 128
					} else {
						block[j*8+i] = float64(ycbcr[y*w+x].Y)
					}
				}
			}
			dct8(&block)
			for _, k := range midFreqIdx {
				bits = append(bits, qimExtract(block[k], float64(q)))
			}
		}
	}
	return bits, nil
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
