package steg

import (
	"crypto/sha256"

	"steggo/pkg/crypto"
)

// Xorshift64Star 是一个确定性的伪随机数生成器。
// 保证跨平台、跨版本生成完全一致的坐标序列（抗检测 LSB 核心依赖）。
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

// SeedFromPassword 由用户密码 + 固定盐派生隐写坐标随机种子。
// 相同密码必然得到相同坐标序列；密码错误则坐标序列完全不同。
func SeedFromPassword(password []byte) []byte {
	salt := []byte(crypto.FixedSalt)
	return crypto.DeriveKey(password, salt, crypto.DefaultIterations)
}

// PixelCursor 负责生成去重后的伪随机像素坐标序列。
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
