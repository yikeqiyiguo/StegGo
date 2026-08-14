package algorithm

import (
	"errors"
	"fmt"
	"image"
	"math"
)

// adaptive 失真成本自适应 LSB 算法组（HUGO / WOW / UNIWARD 简化实现）。
//
// 核心思想：成本函数度量每个像素的"复杂程度"，复杂（纹理/边缘）区域嵌入
// 不易被察觉，平滑区域嵌入代价高。嵌入时以概率 ∝ 成本抽样像素（复杂优先），
// 数据集中在高复杂度区域，抗统计检测能力优于均匀游走 LSB。
//
// 关键设计：成本函数只读取像素高位（忽略低 2 位），嵌入仅修改低 1 位，
// 因此嵌入前后成本图完全一致 ⇒ 抽样序列严格一致 ⇒ 盲提取无漂移。
//
// 说明：为工程可行性，本实现为学术算法的简化版（HILL 风格成本 + 拒绝采样），
// 未实现校验子编码 STC；保真度与安全性优于基础 LSB，足以支撑三端功能。
type adaptive struct {
	id   byte
	name string
	cost costFn
}

// NewHUGO 创建 HUGO 风格自适应算法。
func NewHUGO() Algorithm { return newAdaptive(IDHUGO, "hugo", hillCost) }

// NewWOW 创建 WOW 风格自适应算法。
func NewWOW() Algorithm { return newAdaptive(IDWOW, "wow", wowCost) }

// NewUNIWARD 创建 UNIWARD 风格自适应算法。
func NewUNIWARD() Algorithm { return newAdaptive(IDUNIWARD, "uniward", uniwardCost) }

func newAdaptive(id byte, name string, cost costFn) Algorithm {
	return &adaptive{id: id, name: name, cost: cost}
}

func (a *adaptive) ID() byte     { return a.id }
func (a *adaptive) Name() string { return a.name }

// costFor 根据成本函数名解析成本函数；空名或未知名返回算法默认成本。
// 使嵌入/提取与提取扫描矩阵（costStyle=hill|wow|uniward）语义统一，
// 避免"扫描矩阵成本名与算法内部成本脱节"导致提取失败。
func (a *adaptive) costFor(name string) costFn {
	switch name {
	case "hill":
		return hillCost
	case "wow":
		return wowCost
	case "uniward":
		return uniwardCost
	}
	return a.cost
}

// adaptiveLowBits 成本忽略的像素低位（嵌入 1 bit 不影响成本）。
const adaptiveLowBits = 2

// Capacity 返回最大可嵌入位数（1 bit × 3 通道 × 像素数）。
func (a *adaptive) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	return b.Dx() * b.Dy() * 3
}

// Embed 将位流按成本加权游走写入图像。
func (a *adaptive) Embed(img *image.NRGBA, bits []byte, opt Options) error {
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
	hi := newImageNRGBAHi(img, adaptiveLowBits)
	cursor := NewCostCursor(opt.Seed, w, h, hi, a.costFor(opt.CostStyle))
	pos := 0

	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		ch := &c
		ch.R = writeChannelBits(ch.R, 1, func() byte { return nextBit(bits, &pos, cursor.rng) })
		ch.G = writeChannelBits(ch.G, 1, func() byte { return nextBit(bits, &pos, cursor.rng) })
		ch.B = writeChannelBits(ch.B, 1, func() byte { return nextBit(bits, &pos, cursor.rng) })
		img.SetNRGBA(x, y, c)
	}
	if pos < len(bits) {
		return fmt.Errorf("嵌入容量不足: 成本过滤后实际可用 %d 位, 需要 %d 位", pos, len(bits))
	}
	return nil
}

// Extract 按相同成本加权游走读取位流。
func (a *adaptive) Extract(img *image.NRGBA, opt Options) ([]byte, error) {
	if img == nil {
		return nil, errors.New("图像为空")
	}
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	hi := newImageNRGBAHi(img, adaptiveLowBits)
	cursor := NewCostCursor(opt.Seed, w, h, hi, a.costFor(opt.CostStyle))

	bits := make([]byte, 0, w*h*3)
	for {
		_, x, y := cursor.Next()
		if x < 0 {
			break
		}
		c := img.NRGBAAt(x, y)
		bits = append(bits, c.R&1, c.G&1, c.B&1)
	}
	return bits, nil
}

// =============================================================
// 成本函数（全部基于灰度高位，对 ±1 嵌入不敏感）
// =============================================================

// hillCost HILL 风格：rho = 1 / (lowpass(|highpass(X)|) + ε)。
// 平滑区域 highpass≈0 ⇒ 成本大 ⇒ 抽样概率低。
func hillCost(x, y int, img *imageNRGBAHi) float64 {
	sm := blurAbs(x, y, img)
	return 1.0 / (sm + 1e-9)
}

// blur3 3×3 邻域灰度均值。
func blur3(x, y int, img *imageNRGBAHi) float64 {
	sum := float64(0)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sum += float64(img.grayAt(x+dx, y+dy))
		}
	}
	return sum / 9
}

// blurAbs 3×3 邻域 |g - blur3| 均值（HILL 的第二重低通）。
func blurAbs(x, y int, img *imageNRGBAHi) float64 {
	center := blur3(x, y, img)
	sum := float64(0)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sum += math.Abs(float64(img.grayAt(x+dx, y+dy)) - center)
		}
	}
	return sum / 9
}

// wowCost WOW 风格（简化）：水平与垂直方向高通残差取较小值。
// 边缘/纹理方向性强的区域成本低（适合嵌入）。
func wowCost(x, y int, img *imageNRGBAHi) float64 {
	g := float64(img.grayAt(x, y))
	hl := float64(img.grayAt(x-1, y))
	hr := float64(img.grayAt(x+1, y))
	vu := float64(img.grayAt(x, y-1))
	vd := float64(img.grayAt(x, y+1))
	hRes := math.Abs(g-hl) + math.Abs(g-hr)
	vRes := math.Abs(g-vu) + math.Abs(g-vd)
	return 1.0 / (math.Min(hRes, vRes) + 1e-9)
}

// uniwardCost UNIWARD 风格（简化）：4×4 块内行列差分能量。
func uniwardCost(x, y int, img *imageNRGBAHi) float64 {
	e := float64(0)
	for j := -2; j < 2; j++ {
		for i := -2; i < 2; i++ {
			g := float64(img.grayAt(x+i, y+j))
			dl := float64(img.grayAt(x+i-1, y+j))
			du := float64(img.grayAt(x+i, y+j-1))
			e += (g-dl)*(g-dl) + (g-du)*(g-du)
		}
	}
	return 1.0 / (e + 1e-9)
}
