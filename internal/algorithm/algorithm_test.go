package algorithm

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// makeTestImage 生成合成测试图像：渐变背景 + 平滑区 + 强纹理区。
func makeTestImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// 左半渐变，右半棋盘纹理，含平滑区
			v := uint8(x * 255 / w)
			if x > w*3/4 {
				v = uint8(200 + (x % 20)) // 平滑区
			}
			texture := uint8(0)
			if x > w/4 && x <= w*3/4 && ((x/4+y/4)%2 == 0) {
				texture = 60
			}
			img.SetNRGBA(x, y, color.NRGBA{R: v + texture, G: 100 + texture/2, B: 255 - v, A: 255})
		}
	}
	return img
}

// makeDataBits 生成确定性 0/1 位流。
func makeDataBits(n int) []byte {
	bits := make([]byte, n)
	r := rand.New(rand.NewSource(42))
	for i := range bits {
		bits[i] = byte(r.Intn(2))
	}
	return bits
}

func TestRegisterAndGet(t *testing.T) {
	names := Names()
	// 内置算法固定在前，其余注册算法（anchored 等）按名称排序在后。
	want := []string{"lsb", "dct", "dwt", "hugo", "wow", "uniward", "anchored"}
	if len(names) != len(want) {
		t.Fatalf("Names() = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("Names() = %v, want %v", names, want)
		}
		if Get(n) == nil {
			t.Fatalf("Get(%q) = nil", n)
		}
	}
	if Get("nonexistent") != nil {
		t.Fatal("Get(nonexistent) should be nil")
	}
	// 按 ID 查
	for id := byte(0); id <= IDUNIWARD; id++ {
		if GetByID(id) == nil {
			t.Fatalf("GetByID(%d) = nil", id)
		}
	}
}

func TestLSBRoundtrip(t *testing.T) {
	img := makeTestImage(64, 64)
	seed := []byte("test-seed-lsb")
	for _, depth := range []int{1, 2, 3, 4} {
		for _, mask := range []int{0b111, 0b101, 0b010} {
			opt := Options{BitDepth: depth, ChannelMask: mask, Seed: seed}
			a := NewLSB()
			cap := a.Capacity(img, opt)
			if cap <= 0 {
				t.Fatalf("depth=%d mask=%b: capacity=%d", depth, mask, cap)
			}
			// 嵌入 80% 容量
			n := cap * 4 / 5
			bits := makeDataBits(n)
			work := cloneNRGBA(img)
			if err := a.Embed(work, bits, opt); err != nil {
				t.Fatalf("depth=%d mask=%b: embed: %v", depth, mask, err)
			}
			got, err := a.Extract(work, opt)
			if err != nil {
				t.Fatalf("depth=%d mask=%b: extract: %v", depth, mask, err)
			}
			if !bytes.Equal(got[:n], bits) {
				// 找第一个不同位
				diff := -1
				for i := 0; i < n; i++ {
					if got[i] != bits[i] {
						diff = i
						break
					}
				}
				t.Fatalf("depth=%d mask=%b: data mismatch at bit %d", depth, mask, diff)
			}
		}
	}
}

func TestDCTRoundtrip(t *testing.T) {
	img := makeTestImage(64, 64)
	seed := []byte("test-seed-dct")
	for _, q := range []int{8, 16} {
		opt := Options{Quality: q, Seed: seed}
		a := NewDCT()
		cap := a.Capacity(img, opt)
		if cap < 100 {
			t.Fatalf("q=%d: capacity too small: %d", q, cap)
		}
		n := cap * 3 / 4
		bits := makeDataBits(n)
		work := cloneNRGBA(img)
		if err := a.Embed(work, bits, opt); err != nil {
			t.Fatalf("q=%d: embed: %v", q, err)
		}
		got, err := a.Extract(work, opt)
		if err != nil {
			t.Fatalf("q=%d: extract: %v", q, err)
		}
		mismatch := 0
		for i := 0; i < n; i++ {
			if got[i] != bits[i] {
				mismatch++
			}
		}
		// 允许少量边界误差（YCbCr 舍入临界），但应 <1%
		if mismatch*100 > n {
			t.Fatalf("q=%d: too many mismatches %d/%d", q, mismatch, n)
		}
		psnr := PSNR(img, work)
		if psnr < 25 {
			t.Fatalf("q=%d: PSNR too low: %.1f dB", q, psnr)
		}
		t.Logf("q=%d: mismatches=%d/%d PSNR=%.1f dB", q, mismatch, n, psnr)
	}
}

func TestDWTRoundtrip(t *testing.T) {
	img := makeTestImage(64, 64)
	seed := []byte("test-seed-dwt")
	for _, levels := range []int{1, 2, 3} {
		opt := Options{Levels: levels, Seed: seed}
		a := NewDWT()
		cap := a.Capacity(img, opt)
		if cap < 100 {
			t.Fatalf("levels=%d: capacity too small: %d", levels, cap)
		}
		n := cap * 3 / 4
		bits := makeDataBits(n)
		work := cloneNRGBA(img)
		if err := a.Embed(work, bits, opt); err != nil {
			t.Fatalf("levels=%d: embed: %v", levels, err)
		}
		got, err := a.Extract(work, opt)
		if err != nil {
			t.Fatalf("levels=%d: extract: %v", levels, err)
		}
		if !bytes.Equal(got[:n], bits) {
			diff := -1
			for i := 0; i < n; i++ {
				if got[i] != bits[i] {
					diff = i
					break
				}
			}
			t.Fatalf("levels=%d: data mismatch at bit %d", levels, diff)
		}
		psnr := PSNR(img, work)
		t.Logf("levels=%d: cap=%d PSNR=%.1f dB", levels, cap, psnr)
	}
}

func TestAdaptiveRoundtrip(t *testing.T) {
	seed := []byte("test-seed-adapt")
	for _, al := range []Algorithm{NewHUGO(), NewWOW(), NewUNIWARD()} {
		img := makeTestImage(64, 64)
		opt := Options{Seed: seed}
		cap := al.Capacity(img, opt)
		if cap < 100 {
			t.Fatalf("%s: capacity too small: %d", al.Name(), cap)
		}
		// 成本过滤后实际容量低于理论值，用保守比例
		n := cap * 2 / 5
		bits := makeDataBits(n)
		work := cloneNRGBA(img)
		if err := al.Embed(work, bits, opt); err != nil {
			t.Fatalf("%s: embed: %v", al.Name(), err)
		}
		got, err := al.Extract(work, opt)
		if err != nil {
			t.Fatalf("%s: extract: %v", al.Name(), err)
		}
		if !bytes.Equal(got[:n], bits) {
			diff := -1
			for i := 0; i < n; i++ {
				if got[i] != bits[i] {
					diff = i
					break
				}
			}
			t.Fatalf("%s: data mismatch at bit %d", al.Name(), diff)
		}
		psnr := PSNR(img, work)
		t.Logf("%s: cap=%d PSNR=%.1f dB", al.Name(), cap, psnr)
	}
}

func TestCapacityOverflow(t *testing.T) {
	img := makeTestImage(16, 16)
	seed := []byte("overflow")
	opt := Options{BitDepth: 1, Seed: seed}
	a := NewLSB()
	cap := a.Capacity(img, opt)
	if err := a.Embed(cloneNRGBA(img), makeDataBits(cap+1), opt); err == nil {
		t.Fatal("expected capacity error")
	}
}

func TestAnalyze(t *testing.T) {
	img := makeTestImage(32, 32)
	a := NewLSB()
	opt := Options{BitDepth: 1, Seed: []byte("analyze")}
	res := Analyze(img, a, opt)
	if res.CapacityBytes <= 0 {
		t.Fatalf("capacity bytes = %d", res.CapacityBytes)
	}
	if res.Width != 32 || res.Height != 32 {
		t.Fatalf("size = %dx%d", res.Width, res.Height)
	}
	if res.ChiSquare < 0 || res.ChiSquare > 1 {
		t.Fatalf("chisquare = %v", res.ChiSquare)
	}

	// 嵌入后对比
	work := cloneNRGBA(img)
	bits := makeDataBits(res.CapacityBytes * 8)
	if err := a.Embed(work, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}
	cmp := Compare(img, work, a, opt)
	if cmp.PSNR == 0 || cmp.SSIM == 0 {
		t.Fatalf("PSNR=%v SSIM=%v", cmp.PSNR, cmp.SSIM)
	}
	if cmp.SSIM > 1 || cmp.PSNR <= 0 {
		t.Fatal("unexpected metric")
	}
	t.Logf("PSNR=%.1f dB SSIM=%.4f", cmp.PSNR, cmp.SSIM)
}

func TestChiSquareDetectsStego(t *testing.T) {
	// 卡方攻击针对灰度值 LSB 直接替换：
	// 随机化灰度 LSB 后奇偶直方图趋于均衡，p 值升高。
	img := makeNaturalImage(64, 64)
	before := ChiSquare(img)

	// 手动灰度 LSB 替换（模拟灰度图 LSB 隐写）
	r := rand.New(rand.NewSource(99))
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			v := int(c.R) // 灰度图 R=G=B
			v = (v &^ 1) | r.Intn(2)
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(v), G: uint8(v), B: uint8(v), A: 255})
		}
	}
	after := ChiSquare(img)
	t.Logf("chisquare before=%.4f after=%.4f", before, after)
	if after <= before {
		t.Fatalf("chisquare should rise after LSB randomization: before=%v after=%v", before, after)
	}
}

// makeNaturalImage 生成近似自然照片的图像（随机噪声 + 高斯模糊）。
func makeNaturalImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(7))
	// 随机噪声
	vals := make([][]float64, h)
	for y := 0; y < h; y++ {
		vals[y] = make([]float64, w)
		for x := 0; x < w; x++ {
			vals[y][x] = float64(r.Intn(256))
		}
	}
	// 3×3 均值模糊 2 次
	blur := func(src [][]float64) [][]float64 {
		dst := make([][]float64, h)
		for y := 0; y < h; y++ {
			dst[y] = make([]float64, w)
			for x := 0; x < w; x++ {
				sum, cnt := 0.0, 0.0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						xx, yy := x+dx, y+dy
						if xx >= 0 && xx < w && yy >= 0 && yy < h {
							sum += src[yy][xx]
							cnt++
						}
					}
				}
				dst[y][x] = sum / cnt
			}
		}
		return dst
	}
	vals = blur(vals)
	vals = blur(vals)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(vals[y][x])
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

func TestByteBitConversion(t *testing.T) {
	data := []byte{0x00, 0xFF, 0x55, 0xAA, 0x12}
	bits := ByteToBits(data)
	if len(bits) != len(data)*8 {
		t.Fatalf("bits len = %d", len(bits))
	}
	back := BitsToBytes(bits)
	if !bytes.Equal(back, data) {
		t.Fatalf("roundtrip: %x != %x", back, data)
	}
	// 验证 MSB-first
	if bits[0] != 0 || bits[7] != 0 {
		t.Fatalf("0x00 bits = %v", bits[:8])
	}
	if bits[8] != 1 || bits[15] != 1 {
		t.Fatalf("0xFF bits = %v", bits[8:16])
	}
}

func TestRNGDeterminism(t *testing.T) {
	a := NewRNG([]byte("seed"))
	b := NewRNG([]byte("seed"))
	for i := 0; i < 100; i++ {
		if a.Next() != b.Next() {
			t.Fatal("RNG not deterministic")
		}
	}
	// 不同种子不同序列
	c := NewRNG([]byte("seed2"))
	if a.Next() == c.Next() {
		t.Fatal("different seeds produced same value")
	}
}

func TestPixelCursorCoverage(t *testing.T) {
	// 游标应完整覆盖所有像素且不重复
	w, h := 32, 32
	c := NewPixelCursor([]byte("cover"), w, h)
	seen := make([]bool, w*h)
	count := 0
	for {
		idx, _, _ := c.Next()
		if idx < 0 {
			break
		}
		if seen[idx] {
			t.Fatal("duplicate pixel")
		}
		seen[idx] = true
		count++
	}
	if count != w*h {
		t.Fatalf("covered %d/%d", count, w*h)
	}
}

func TestDWTInverseIdentity(t *testing.T) {
	// 整数 Haar 逆变换应完全还原
	plane := make([]int, 64*64)
	for i := range plane {
		plane[i] = i * 3 % 256
	}
	orig := append([]int(nil), plane...)
	bands := decomposeHaar(plane, 64, 64, 2)
	recomposeHaar(plane, 64, 64, bands, 2)
	if !intsEqual(plane, orig) {
		t.Fatal("DWT inverse not identity")
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneNRGBA(img *image.NRGBA) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	copy(out.Pix, img.Pix)
	return out
}

func TestDCTCapacityFormula(t *testing.T) {
	img := makeTestImage(64, 64)
	a := NewDCT()
	opt := Options{Quality: 8}
	// 64x64 => 8x8 块 = 64 块，每块 24 系数
	if want := 64 * coefPerBlock; a.Capacity(img, opt) != want {
		t.Fatalf("DCT capacity = %d, want %d", a.Capacity(img, opt), want)
	}
}

func TestOptionsValidation(t *testing.T) {
	a := NewLSB()
	img := makeTestImage(8, 8)
	badOpts := []Options{
		{BitDepth: 5, Seed: []byte("x")},            // 超出 1-4
		{BitDepth: -1, Seed: []byte("x")},           // 非法
		{ChannelMask: 0b1000, Seed: []byte("x")},    // 超出 3 通道
		{CostStyle: "unknown", Seed: []byte("x")},   // 未知成本函数
		{BitDepth: 2, Levels: 4, Seed: []byte("x")}, // DWT 级数超出
	}
	for _, opt := range badOpts {
		if err := a.Embed(cloneNRGBA(img), makeDataBits(4), opt); err == nil {
			t.Fatalf("expected error for %+v", opt)
		}
	}
	if err := (&Options{BitDepth: 2, Seed: []byte("x")}).fillDefaults(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
}

// 示例：如何在字节与位之间配合算法使用。
func Example_lsbEmbed() {
	img := makeTestImage(16, 16)
	a := NewLSB()
	opt := Options{BitDepth: 2, Seed: []byte("my-password-derived-seed")}
	msg := []byte("hello")
	bits := ByteToBits(msg)
	if err := a.Embed(img, bits, opt); err != nil {
		fmt.Println("embed failed:", err)
		return
	}
	got, _ := a.Extract(img, opt)
	out := BitsToBytes(got[:len(bits)])
	fmt.Println(string(out))
	// Output: hello
}
