package algorithm

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"testing"

	"steggo/internal/vision"
)

// testAnchoredImage 生成 512×512 纹理丰富的测试图（保证 FAST 角点充足）。
func testAnchoredImage(seed int64) *image.NRGBA {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	// 伪随机纹理：随机色块
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			v := uint8(rng.Intn(256))
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	// 叠加棋盘与边缘，增强角点结构
	for y := 0; y < 512; y += 4 {
		for x := 0; x < 512; x += 4 {
			if (x/4+y/4)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
	}
	return img
}

// anchoredOpts 构造测试 Options。
func anchoredOpts() Options {
	return Options{BitDepth: 1, Seed: []byte("anchored-test-seed")}
}

func TestAnchoredRoundTrip(t *testing.T) {
	img := testAnchoredImage(42)
	opt := anchoredOpts()
	a := NewAnchored()

	capBits := a.Capacity(img, opt)
	if capBits < 64 {
		t.Fatalf("capacity too small: %d bits", capBits)
	}
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i*7 + 3)
	}
	bits := ByteToBits(payload)

	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}
	out, err := a.Extract(img, opt)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(out) < len(bits) {
		t.Fatalf("output too short: %d < %d", len(out), len(bits))
	}
	if !bytes.Equal(out[:len(bits)], bits) {
		t.Fatalf("round-trip mismatch: got %d bits, want %d bits", len(out), len(bits))
	}
}

func TestAnchoredRotations(t *testing.T) {
	opt := anchoredOpts()
	a := NewAnchored()
	img := testAnchoredImage(7)
	capBits := a.Capacity(img, opt)
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i * 13)
	}
	bits := ByteToBits(payload)
	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}

	for _, d := range []int{90, 180, 270} {
		rot := rotateNRGBA(img, d)
		out, err := a.Extract(rot, opt)
		if err != nil {
			t.Fatalf("extract after %d° rotation: %v", d, err)
		}
		if !bytes.Equal(out[:len(bits)], bits) {
			t.Fatalf("round-trip mismatch after %d° rotation", d)
		}
	}
}

func TestAnchoredCrop(t *testing.T) {
	opt := anchoredOpts()
	a := NewAnchored()
	img := testAnchoredImage(99)
	capBits := a.Capacity(img, opt)
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i * 29)
	}
	bits := ByteToBits(payload)
	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}

	// 裁掉左下角约 25%（模拟平台裁剪/加水印遮挡）。
	cropped := cropNRGBA(img, 0, 0, 384, 384)
	out, err := a.Extract(cropped, opt)
	if err != nil {
		t.Fatalf("extract after crop: %v", err)
	}
	if !bytes.Equal(out[:len(bits)], bits) {
		t.Fatalf("round-trip mismatch after crop")
	}
}

func TestAnchoredJPEGCompression(t *testing.T) {
	opt := anchoredOpts()
	a := NewAnchored()
	img := testAnchoredImage(2026)
	capBits := a.Capacity(img, opt)
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i*31 + 1)
	}
	bits := ByteToBits(payload)
	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}

	// 模拟社交平台重压缩：JPEG q70。
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatalf("jpeg decode: %v", err)
	}
	compressed := toNRGBA(dec)

	out, err := a.Extract(compressed, opt)
	if err != nil {
		t.Fatalf("extract after jpeg q70: %v", err)
	}
	// 允许少量位错误：JPEG 压缩是尽力而为，要求 ≥95% 位一致。
	diff := 0
	for i := range bits {
		if out[i] != bits[i] {
			diff++
		}
	}
	rate := 1 - float64(diff)/float64(len(bits))
	if rate < 0.95 {
		t.Fatalf("jpeg q70 bit accuracy too low: %.1f%% (%d/%d bits wrong)", rate*100, diff, len(bits))
	}
}

func TestAnchoredCombinedAttack(t *testing.T) {
	opt := anchoredOpts()
	a := NewAnchored()
	img := testAnchoredImage(555)
	capBits := a.Capacity(img, opt)
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i*17 + 5)
	}
	bits := ByteToBits(payload)
	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}

	// 组合攻击：旋转 90° + 裁剪右下角 + JPEG q75。
	rot := rotateNRGBA(img, 90)
	cropped := cropNRGBA(rot, 0, 0, 400, 420)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatalf("jpeg decode: %v", err)
	}
	final := toNRGBA(dec)

	out, err := a.Extract(final, opt)
	if err != nil {
		t.Fatalf("extract after combined attack: %v", err)
	}
	diff := 0
	for i := range bits {
		if out[i] != bits[i] {
			diff++
		}
	}
	rate := 1 - float64(diff)/float64(len(bits))
	if rate < 0.90 {
		t.Fatalf("combined attack bit accuracy too low: %.1f%%", rate*100)
	}
}

// TestAnchoredNoiseImage 纯随机噪声图回归：噪声图的 DWT 高频系数极大，
// QIM 修正经逆变换后像素频繁越界，clamp 截断使交替投影在不相交约束集间
// 振荡发散（不动点陷阱）。修复采用多起点扰动 + 近边界系数中心化后全块收敛。
// 本测试锁定该场景，防止 POCS 优化回归。
func TestAnchoredNoiseImage(t *testing.T) {
	opt := anchoredOpts()
	a := NewAnchored()
	img := makeNoiseImage(512, 512, 7) // 与服务层噪声测试一致
	capBits := a.Capacity(img, opt)
	payload := make([]byte, capBits/8-8)
	for i := range payload {
		payload[i] = byte(i*31 + 1)
	}
	bits := ByteToBits(payload)
	if err := a.Embed(img, bits, opt); err != nil {
		t.Fatalf("embed: %v", err)
	}

	// 无损 PNG 往返：必须 100% 精确。
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	dec, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	round := toNRGBA(dec)
	out, err := a.Extract(round, opt)
	if err != nil {
		t.Fatalf("extract after png roundtrip: %v", err)
	}
	if !bytes.Equal(out[:len(bits)], bits) {
		t.Fatal("png roundtrip bit mismatch")
	}

	// JPEG q75 有损：噪声图是 JPEG 误差放大的极端载体，要求 ≥98% 位一致。
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, round, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	jdec, err := jpeg.Decode(&jbuf)
	if err != nil {
		t.Fatalf("jpeg decode: %v", err)
	}
	out2, err := a.Extract(toNRGBA(jdec), opt)
	if err != nil {
		t.Fatalf("extract after jpeg q75: %v", err)
	}
	diff := 0
	for i := range bits {
		if out2[i] != bits[i] {
			diff++
		}
	}
	rate := 1 - float64(diff)/float64(len(bits))
	if rate < 0.98 {
		t.Fatalf("noise jpeg q75 accuracy too low: %.1f%% (%d/%d bits wrong)", rate*100, diff, len(bits))
	}
}

// makeNoiseImage 生成 w×h 纯随机噪声图（POCS 收敛最难的载体类型）。
func makeNoiseImage(w, h int, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(seed))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(rnd.Intn(256))
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// ---- 测试辅助 ----

// rotateNRGBA 顺时针旋转 d 度（90° 倍数），返回新图。
func rotateNRGBA(img *image.NRGBA, d int) *image.NRGBA {
	gray := vision.ToGray(img)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	rg, nw, nh := vision.Rotate90(gray, w, h, d)
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			v := rg[y*nw+x]
			out.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return out
}

// cropNRGBA 截取 (x0,y0)-(x1,y1) 区域。
func cropNRGBA(img *image.NRGBA, x0, y0, x1, y1 int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := 0; y < y1-y0; y++ {
		for x := 0; x < x1-x0; x++ {
			out.SetNRGBA(x, y, img.NRGBAAt(x+x0, y+y0))
		}
	}
	return out
}

// toNRGBA 任意图像转 NRGBA。
func toNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(x+b.Min.X, y+b.Min.Y)).(color.NRGBA))
		}
	}
	return out
}
