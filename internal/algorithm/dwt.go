package algorithm

import (
	"errors"
	"fmt"
	"image"
)

// dwt 整数 Haar 小波（S 变换提升）频域隐写。
//
// 流程：RGB → YCbCr → 对 Y 平面做 levels 级 2D 整数 Haar 分解 →
// 在各级高频细节系数（HL/LH/HH）做 ±1 奇偶嵌入 → 逆变换还原。
//
// 整数提升完全可逆 ⇒ 嵌入后逆变换再正变换得到的系数与嵌入时一致，
// 盲提取读取系数奇偶即得位流，往返无损。
type dwt struct{}

// NewDWT 创建 DWT 算法实例。
func NewDWT() Algorithm { return &dwt{} }

func (a *dwt) ID() byte     { return IDDWT }
func (a *dwt) Name() string { return "dwt" }

// dwtBand 一级分解的子带。
type dwtBand struct {
	w, h   int
	coeff  []int
	origin [2]int // 子带左上角在原平面中的坐标
}

// Capacity 返回最大可嵌入位数（各级 HL/LH/HH 系数总数）。
func (a *dwt) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	if err := opt.fillDefaults(); err != nil {
		return 0
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	total := 0
	cw, ch := w, h
	for l := 0; l < opt.Levels; l++ {
		hw, hh := cw/2, ch/2
		total += 3 * hw * hh // HL + LH + HH
		cw, ch = hw, hh
	}
	return total
}

// dwtQuant 小波系数量化间隔（中心化 QIM，容差 = dwtQuant/2）。
// 需覆盖：YCbCr 往返舍入误差（≤1）+ 逆变换 clamp 误差（≤1）。
const dwtQuant = 4

// dwtYLo/dwtYHi Y 平面值域压缩边界（仿 JPEG），防止逆变换越界 clamp 丢失信息。
const (
	dwtYLo = 16
	dwtYHi = 239
)

// compressY 将 Y 从 [0,255] 线性压缩到 [16,239]，为高频系数 ±2 修改预留边界。
func compressY(y uint8) int {
	return dwtYLo + int(y)*(dwtYHi-dwtYLo)/255
}

// Embed 将位流嵌入小波高频系数。
// Y 平面先压缩到 [16,239]，系数用 QIM（间隔 dwtQuant）中心化嵌入。
//
// 采用 POCS 迭代：每次“整数像素平面 → 分解 → 嵌入 → 逆变换”，
// 再经 RGB 往返（roundTripY 模拟提取端真实读取链路）验证系数桶，
// 不满足则以读回的 Y 平面为新的起点继续投影，直到收敛。
// 这保证“写入像素 → 提取读取”整条链路可逆——整数提升虽可逆，
// 但 fromYCbCr→toYCbCr 的舍入会把误差放大到小波系数桶，必须显式验证。
func (a *dwt) Embed(img *image.NRGBA, bits []byte, opt Options) error {
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

	ycbcr := toYCbCr(img)
	cbs := make([]uint8, w*h)
	crs := make([]uint8, w*h)
	plane := make([]int, w*h)
	for i := range plane {
		plane[i] = compressY(ycbcr[i].Y)
		cbs[i] = ycbcr[i].Cb
		crs[i] = ycbcr[i].Cr
	}

	out, ok := embedPlanePOCS(plane, cbs, crs, w, h, opt, bits)
	if !ok {
		return fmt.Errorf("DWT 嵌入 POCS 未收敛，请尝试降低 Levels（当前 %d）", opt.Levels)
	}
	for i := range out {
		ycbcr[i].Y = clampByte(out[i])
	}
	fromYCbCr(img, ycbcr)
	return nil
}

// embedPlanePOCS 在 Y 平面（压缩域）上做 POCS 迭代嵌入。
// 多起点：先原始平面，再若干确定性小扰动，跳出 RGB 往返不动点陷阱。
func embedPlanePOCS(plane []int, cbs, crs []uint8, w, h int, opt Options, bits []byte) ([]int, bool) {
	starts := [][]int{plane}
	rng := NewRNG([]byte{0xD2, 0x11, 0x22, 0x33})
	for s := 0; s < 5; s++ {
		sp := make([]int, w*h)
		copy(sp, plane)
		for i := range sp {
			d := 0
			for b := 0; b < 3; b++ {
				d += int(rng.NextBit())
			}
			v := sp[i] + d - 1
			if v < 0 {
				v = 0
			}
			if v > 255 {
				v = 255
			}
			sp[i] = v
		}
		starts = append(starts, sp)
	}

	for _, start := range starts {
		cur := make([]int, w*h)
		copy(cur, start)
		for iter := 0; iter < 60; iter++ {
			bands := decomposeHaar(cur, w, h, opt.Levels)
			bi := 0
			rngB := NewRNG(append(opt.Seed, 0x2B))
			for _, band := range bands {
				for i := range band.coeff {
					bit := byte(0)
					if bi < len(bits) {
						bit = bits[bi]
					} else {
						bit = rngB.NextBit()
					}
					bi++
					band.coeff[i] = qimInt(band.coeff[i], bit)
				}
			}
			recomposeHaar(cur, w, h, bands, opt.Levels)

			// 模拟提取端：clamp + RGB 往返后读回的 Y 平面
			readBack := make([]int, w*h)
			for i := range readBack {
				readBack[i] = int(roundTripY(clampByte(cur[i]), uint8(cbs[i]), uint8(crs[i])))
			}

			// 验证数据区系数桶
			bands2 := decomposeHaar(readBack, w, h, opt.Levels)
			ok := true
			bi = 0
			for _, band := range bands2 {
				for _, k := range band.coeff {
					if bi >= len(bits) {
						break
					}
					if qimExtractInt(k) != bits[bi] {
						ok = false
						break
					}
					bi++
				}
				if !ok {
					break
				}
			}
			if ok {
				out := make([]int, w*h)
				for i := range out {
					out[i] = int(clampByte(cur[i]))
				}
				return out, true
			}
			cur = readBack
		}
	}
	return plane, false
}

// Extract 从高频系数读取位流。
// 嵌入后的图像 Y 已处于压缩域，直接分解读取（不再二次压缩）。
func (a *dwt) Extract(img *image.NRGBA, opt Options) ([]byte, error) {
	if img == nil {
		return nil, errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ycbcr := toYCbCr(img)
	plane := make([]int, w*h)
	for i := range plane {
		plane[i] = int(ycbcr[i].Y)
	}
	bands := decomposeHaar(plane, w, h, opt.Levels)

	bits := make([]byte, 0, a.Capacity(img, opt))
	for _, band := range bands {
		for _, k := range band.coeff {
			bits = append(bits, qimExtractInt(k))
		}
	}
	return bits, nil
}

// qimInt QIM 嵌入：将系数推至目标桶中心（容差 dwtQuant/2），桶奇偶编码 bit。
func qimInt(k int, bit byte) int {
	sign := 1
	abs := k
	if k < 0 {
		sign, abs = -1, -k
	}
	target := abs / dwtQuant
	if target&1 != int(bit&1) {
		if target > 0 {
			target--
		} else {
			target++
		}
	}
	return sign * (target*dwtQuant + dwtQuant/2)
}

// qimExtractInt 读取桶奇偶位。
func qimExtractInt(k int) byte {
	abs := k
	if k < 0 {
		abs = -k
	}
	return byte((abs / dwtQuant) & 1)
}

// decomposeHaar 多级 2D 整数 Haar 分解，返回各级 HL/LH/HH 子带引用。
// 分解采用 S 变换（整数提升，完全可逆）。
func decomposeHaar(plane []int, w, h, levels int) []*dwtBand {
	bands := make([]*dwtBand, 0, levels*3)
	cw, ch := w, h
	for l := 0; l < levels; l++ {
		hw, hh := cw/2, ch/2
		if hw == 0 || hh == 0 {
			break
		}
		// 一级分解：对平面左上角 cw×ch 区域
		// 先水平，再垂直
		// 临时：对每行做水平 S 变换，结果写回：左=低通 s，右=高通 d
		rowTmp := make([]int, cw)
		for y := 0; y < ch; y++ {
			base := y * w
			for x := 0; x < cw; x++ {
				rowTmp[x] = plane[base+x]
			}
			for x := 0; x < hw; x++ {
				x0, x1 := rowTmp[2*x], rowTmp[2*x+1]
				s, d := sTransform(x0, x1)
				plane[base+x] = s
				plane[base+hw+x] = d
			}
		}
		// 垂直：对每列做 S 变换
		colTmp := make([]int, ch)
		for x := 0; x < cw; x++ {
			for y := 0; y < ch; y++ {
				colTmp[y] = plane[y*w+x]
			}
			for y := 0; y < hh; y++ {
				y0, y1 := colTmp[2*y], colTmp[2*y+1]
				s, d := sTransform(y0, y1)
				plane[y*w+x] = s
				plane[(hh+y)*w+x] = d
			}
		}
		// 现在平面布局（cw×ch 左上角）：
		//   [0:hw, 0:hh]       = LL
		//   [0:hw, hh:ch]      = HL（垂直高频）
		//   [hw:cw, 0:hh]      = LH（水平高频）
		//   [hw:cw, hh:ch]     = HH
		hlBand := &dwtBand{w: hw, h: hh, origin: [2]int{hw, 0}} // 右上：水平高通
		lhBand := &dwtBand{w: hw, h: hh, origin: [2]int{0, hh}} // 左下：垂直高通
		hhBand := &dwtBand{w: hw, h: hh, origin: [2]int{hw, hh}}
		hlBand.coeff = planeView(plane, w, hlBand)
		lhBand.coeff = planeView(plane, w, lhBand)
		hhBand.coeff = planeView(plane, w, hhBand)
		bands = append(bands, hlBand, lhBand, hhBand)
		// 下一级作用于 LL（左上 hw×hh）
		cw, ch = hw, hh
	}
	return bands
}

// planeView 建立子带系数切片视图（行主序，跳行宽 w）。
func planeView(plane []int, w int, band *dwtBand) []int {
	view := make([]int, band.w*band.h)
	ox, oy := band.origin[0], band.origin[1]
	for y := 0; y < band.h; y++ {
		copy(view[y*band.w:(y+1)*band.w], plane[(oy+y)*w+ox:(oy+y)*w+ox+band.w])
	}
	return view
}

// recomposeHaar 逆变换：把各级子带视图写回平面后逐级逆 S 变换。
func recomposeHaar(plane []int, w, h int, bands []*dwtBand, levels int) {
	// 先把子带视图拷回平面
	for _, band := range bands {
		ox, oy := band.origin[0], band.origin[1]
		for y := 0; y < band.h; y++ {
			copy(plane[(oy+y)*w+ox:(oy+y)*w+ox+band.w], band.coeff[y*band.w:(y+1)*band.w])
		}
	}
	// 从最深层逐级还原
	cw, ch := w, h
	// 计算各级尺寸
	sizes := make([][2]int, 0, levels)
	for l := 0; l < levels; l++ {
		sizes = append(sizes, [2]int{cw, ch})
		cw, ch = cw/2, ch/2
	}
	for l := levels - 1; l >= 0; l-- {
		cw, ch = sizes[l][0], sizes[l][1]
		hw, hh := cw/2, ch/2
		if hw == 0 || hh == 0 {
			continue
		}
		// 先逆垂直，再逆水平。
		// 注意：逆变换的输出 (2i, 2i+1) 会覆盖下一组的输入位置 (i+1, ...)，
		// 必须使用独立输出缓冲，否则原地覆盖导致错误。
		colTmp := make([]int, ch)
		colOut := make([]int, ch)
		for x := 0; x < cw; x++ {
			for y := 0; y < ch; y++ {
				colTmp[y] = plane[y*w+x]
			}
			for y := 0; y < hh; y++ {
				a, b := invSTransform(colTmp[y], colTmp[hh+y])
				colOut[2*y], colOut[2*y+1] = a, b
			}
			for y := 0; y < ch; y++ {
				plane[y*w+x] = colOut[y]
			}
		}
		rowTmp := make([]int, cw)
		rowOut := make([]int, cw)
		for y := 0; y < ch; y++ {
			base := y * w
			for x := 0; x < cw; x++ {
				rowTmp[x] = plane[base+x]
			}
			for x := 0; x < hw; x++ {
				a, b := invSTransform(rowTmp[x], rowTmp[hw+x])
				rowOut[2*x], rowOut[2*x+1] = a, b
			}
			for x := 0; x < cw; x++ {
				plane[base+x] = rowOut[x]
			}
		}
	}
}

// sTransform S 变换（整数 Haar 提升）：
//
//	s = floor((a+b)/2), d = a-b
//	逆：a = s + floor((d+1)/2), b = s - floor(d/2)
func sTransform(a, b int) (s, d int) {
	return (a + b) >> 1, a - b
}

func invSTransform(s, d int) (a, b int) {
	a = s + (d+1)>>1
	b = s - d>>1
	return a, b
}
