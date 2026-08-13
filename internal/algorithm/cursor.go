package algorithm

import (
	"crypto/sha256"
	"image"
)

// Xorshift64Star 确定性伪随机数生成器。
// 跨平台、跨版本产生完全一致的序列（抗检测游走核心依赖）。
type Xorshift64Star struct {
	state uint64
}

// NewRNG 用字节种子初始化 PRNG。
func NewRNG(seed []byte) *Xorshift64Star {
	h := sha256.Sum256(seed)
	var s uint64
	for i := 0; i < 8; i++ {
		s = s<<8 | uint64(h[i])
	}
	if s == 0 {
		s = 0x9E3779B97F4A7C15
	}
	return &Xorshift64Star{state: s}
}

// Next 返回下一个 uint64 伪随机数。
func (r *Xorshift64Star) Next() uint64 {
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 0x2545F4914F6CDD1D
}

// NextBit 返回一个伪随机 bit (0/1)。
func (r *Xorshift64Star) NextBit() byte {
	return byte(r.Next() & 1)
}

// NextInt 返回 [0, n) 范围内的伪随机整数。
func (r *Xorshift64Star) NextInt(n uint64) uint64 {
	if n <= 0 {
		return 0
	}
	return r.Next() % n
}

// PixelCursor 生成去重后的伪随机像素坐标序列。
//
// 采用"伪随机抽样 + 线性探测回退"：先用随机序列打散坐标（抗检测核心），
// 抽样碰撞时按索引顺序回退，保证确定性且完整覆盖全部像素。
// 嵌入与提取使用相同的 rng 序列与调用次数，坐标序列严格一致。
type PixelCursor struct {
	rng  *Xorshift64Star
	w, h int
	used []uint64 // bitset，记录已使用像素
	pos  uint64   // 线性探测游标
}

// NewPixelCursor 创建像素坐标游标。
func NewPixelCursor(seed []byte, w, h int) *PixelCursor {
	total := w * h
	bits := (total + 63) / 64
	return &PixelCursor{
		rng:  NewRNG(seed),
		w:    w,
		h:    h,
		used: make([]uint64, bits),
	}
}

// Next 返回下一个未使用的像素坐标 (idx 索引, x, y)，-1 表示已耗尽。
func (c *PixelCursor) Next() (idx, x, y int) {
	total := c.w * c.h
	// 阶段一：伪随机抽样（最多 8 次）
	for i := 0; i < 8; i++ {
		pos := c.rng.NextInt(uint64(total))
		word, bit := pos/64, pos%64
		if c.used[word]&(1<<bit) == 0 {
			c.used[word] |= 1 << bit
			return int(pos), int(pos % uint64(c.w)), int(pos / uint64(c.w))
		}
	}
	// 阶段二：线性探测回退（确定性）
	for c.pos < uint64(total) {
		p := c.pos
		c.pos++
		word, bit := p/64, p%64
		if c.used[word]&(1<<bit) == 0 {
			c.used[word] |= 1 << bit
			return int(p), int(p % uint64(c.w)), int(p / uint64(c.w))
		}
	}
	return -1, -1, -1
}

// costFn 计算某个像素位置的嵌入成本（嵌入/提取两侧必须严格一致）。
type costFn func(x, y int, img *imageNRGBAHi) float64

// CostCursor 成本加权游走：以概率 ∝ 成本抽样像素（复杂区域优先），
// 拒绝采样保持确定性。嵌入与提取使用相同成本函数 ⇒ 序列严格一致。
//
// 注意：成本必须基于像素高位计算（嵌入低 1-2 位不影响成本），
// 否则嵌入后成本变化会导致提取序列漂移。
type CostCursor struct {
	rng       *Xorshift64Star
	w, h      int
	used      []uint64
	pos       uint64
	cost      costFn
	img       *imageNRGBAHi
	maxCost   float64
	acceptAll bool
}

// NewCostCursor 创建成本加权游标。
func NewCostCursor(seed []byte, w, h int, img *imageNRGBAHi, cost costFn) *CostCursor {
	total := w * h
	bits := (total + 63) / 64
	// 预扫描最大成本用于归一化
	mc := 1.0
	if img != nil && cost != nil {
		if v := scanMaxCost(img, cost); v > mc {
			mc = v
		}
	}
	return &CostCursor{
		rng:     NewRNG(seed),
		w:       w,
		h:       h,
		used:    make([]uint64, bits),
		cost:    cost,
		img:     img,
		maxCost: mc,
	}
}

// setAcceptAll 允许跳过拒绝采样（成本图不可用时全接受）。
func (c *CostCursor) setAcceptAll() { c.acceptAll = true }

// Next 返回下一个被接受的像素坐标；-1 表示耗尽。
func (c *CostCursor) Next() (idx, x, y int) {
	total := c.w * c.h
	// 阶段一：伪随机抽样（最多 8 次），碰撞或被拒绝则继续抽
	for i := 0; i < 8; i++ {
		pos := c.rng.NextInt(uint64(total))
		word, bit := pos/64, pos%64
		if c.used[word]&(1<<bit) != 0 {
			continue // 碰撞：不消耗拒绝随机数，保持两侧一致
		}
		if !c.accept(costVal(c, pos)) {
			continue // 拒绝：accept 内已消耗一个随机数
		}
		c.used[word] |= 1 << bit
		return int(pos), int(pos % uint64(c.w)), int(pos / uint64(c.w))
	}
	// 阶段二：线性探测回退（仍按成本判定，序列一致）
	for c.pos < uint64(total) {
		p := c.pos
		c.pos++
		word, bit := p/64, p%64
		if c.used[word]&(1<<bit) != 0 {
			continue
		}
		if !c.accept(costVal(c, p)) {
			continue
		}
		c.used[word] |= 1 << bit
		return int(p), int(p % uint64(c.w)), int(p / uint64(c.w))
	}
	return -1, -1, -1
}

// acceptMin 拒绝采样保底接受率。
// 成本分布跨度极大（平滑区 1/ε ≈ 1e9），若不保底会近乎全拒导致容量骤减。
// 保底率保证至少 pMin 的候选被接受，同时高成本（复杂区）仍以更高概率入选。
const acceptMin = 0.35

// accept 拒绝采样：接受概率 = pMin + (1-pMin)·cost/maxCost。
// 必须消耗 RNG（保证两侧序列同步）。
func (c *CostCursor) accept(cost float64) bool {
	if c.acceptAll || c.cost == nil || c.img == nil {
		// 仍消耗一个随机数保持同步
		_ = c.rng.Next()
		return true
	}
	p := cost / c.maxCost
	if p > 1 {
		p = 1
	}
	p = acceptMin + (1-acceptMin)*p
	r := float64(c.rng.Next()>>11) / float64(1<<53)
	return r < p
}

// costVal 读取指定像素位置的成本（同一位置两侧一致）。
func costVal(c *CostCursor, pos uint64) float64 {
	if c.cost == nil || c.img == nil {
		return 1
	}
	x, y := int(pos%uint64(c.w)), int(pos/uint64(c.w))
	return c.cost(x, y, c.img)
}

// scanMaxCost 扫描全图成本最大值。
func scanMaxCost(img *imageNRGBAHi, cost costFn) float64 {
	b := img.img.Bounds()
	mx := 1.0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if v := cost(x, y, img); v > mx {
				mx = v
			}
		}
	}
	return mx
}

// imageNRGBAHi 高位图像视图：供成本函数读取像素高位（忽略低 lowBits 位），
// 使成本对 LSB 嵌入不敏感。灰度平面预计算提升成本函数性能。
type imageNRGBAHi struct {
	img     *image.NRGBA
	lowBits int
	gray    []int // 灰度高位平面（行主序，w×h）
	w, h    int
}

func newImageNRGBAHi(img *image.NRGBA, lowBits int) *imageNRGBAHi {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	mask := uint8(0xFF << lowBits)
	gray := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x+b.Min.X, y+b.Min.Y)
			g := (int(c.R&mask) + int(c.G&mask) + int(c.B&mask)) / 3
			gray[y*w+x] = g
		}
	}
	return &imageNRGBAHi{img: img, lowBits: lowBits, gray: gray, w: w, h: h}
}

// grayAt 读取灰度高位（越界 clamp）。
func (h *imageNRGBAHi) grayAt(x, y int) int {
	if x < 0 {
		x = 0
	} else if x >= h.w {
		x = h.w - 1
	}
	if y < 0 {
		y = 0
	} else if y >= h.h {
		y = h.h - 1
	}
	return h.gray[y*h.w+x]
}
