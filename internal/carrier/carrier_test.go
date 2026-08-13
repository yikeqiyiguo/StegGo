package carrier

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"steggo/internal/algorithm"
)

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

// makeTestImage 生成随机测试图像。
func makeTestImage(w, h int, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(seed))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rnd.Intn(256)),
				G: uint8(rnd.Intn(256)),
				B: uint8(rnd.Intn(256)),
				A: 255,
			})
		}
	}
	return img
}

// writeTestPNG 保存测试 PNG。
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := SaveImage(makeTestImage(w, h, 7), path); err != nil {
		t.Fatal(err)
	}
}

func writeTestBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func testOpt(seed string) Options {
	return Options{Algorithm: "lsb", Seed: []byte(seed)}
}

// ---------------------------------------------------------------------------
// 格式识别
// ---------------------------------------------------------------------------

func TestDetectKind(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		data []byte
		want Kind
		ok   bool
	}{
		{"a.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}, KindImage, true},
		{"a.bmp", []byte{'B', 'M', 0, 0, 0, 0, 0, 0}, KindImage, true},
		{"a.tiff", []byte{'I', 'I', 0x2A, 0x00, 0, 0, 0, 0}, KindImage, true},
		{"a.wav", []byte("RIFF\x24\x00\x00\x00WAVEfmt "), KindAudio, true},
		{"a.flac", []byte("fLaC\x00\x00\x00\x22"), KindAudio, true},
		{"a.pdf", []byte("%PDF-1.4\n%%EOF\n"), KindPDF, true},
		{"a.mp4", []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}, KindVideo, true},
		{"a.mkv", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, KindVideo, true},
		{"a.txt", []byte("hello world\n"), KindText, true},
		{"a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10}, KindUnknown, false},
		{"a.mp3", []byte("ID3\x04\x00\x00\x00"), KindUnknown, false},
		{"a.gif", []byte("GIF89a"), KindUnknown, false},
		{"a.xyz", []byte("random bytes here"), KindUnknown, false},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name)
		writeTestBytes(t, path, c.data)
		got, err := DetectKind(path)
		if c.ok {
			if err != nil {
				t.Errorf("%s: 期望成功, 得到 %v", c.name, err)
				continue
			}
			if got != c.want {
				t.Errorf("%s: 期望 %v, 实际 %v", c.name, c.want, got)
			}
		} else {
			if err == nil {
				t.Errorf("%s: 应当返回错误, 得到 %v", c.name, got)
			}
		}
	}
}

func TestDetectKindMagicPriority(t *testing.T) {
	dir := t.TempDir()
	// 伪装扩展名：内容为 PNG，扩展名 .jpg → 按魔数放行（真实格式无损）。
	disguised := filepath.Join(dir, "fake.jpg")
	writeTestBytes(t, disguised, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	if kind, err := DetectKind(disguised); err != nil || kind != KindImage {
		t.Errorf("伪装 .jpg 实际 PNG 应识别为 image, 得到 %v, %v", kind, err)
	}
	// 伪装扩展名：内容为 JPEG，扩展名 .png → 必须拦截（有损魔数优先）。
	fakePNG := filepath.Join(dir, "fake.png")
	writeTestBytes(t, fakePNG, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10})
	if _, err := DetectKind(fakePNG); err == nil {
		t.Error("伪装 .png 实际 JPEG 应被拦截")
	}
}

func TestForPath(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "x.png")
	writeTestPNG(t, png, 16, 16)
	c, err := ForPath(png)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind() != KindImage {
		t.Fatalf("ForPath PNG 应为 image, 得到 %v", c.Kind())
	}
	// 有损格式 ForPath 报错
	bad := filepath.Join(dir, "y.mp3")
	writeTestBytes(t, bad, []byte("ID3\x04\x00\x00\x00"))
	if _, err := ForPath(bad); err == nil {
		t.Fatal("ForPath MP3 应报错")
	}
}

// ---------------------------------------------------------------------------
// 图像载体
// ---------------------------------------------------------------------------

func TestImageCarrierRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	out := filepath.Join(dir, "out.png")
	writeTestPNG(t, src, 96, 96)

	payload := []byte("StegGo V2.0 图像载体测试 payload 1234567890")
	opt := testOpt("carrier-image-seed")
	c := Get(KindImage)
	if c == nil {
		t.Fatal("图像载体未注册")
	}
	if err := c.Embed(src, out, payload, opt); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	got, err := c.Extract(out, opt)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// 提取流头部应包含完整 payload
	if !bytes.HasPrefix(got, payload) {
		t.Fatalf("payload 不一致: 期望 %q, 实际前 %d 字节 %q", payload, len(payload), got[:min(len(got), len(payload))])
	}
}

func TestImageCarrierDCTRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	out := filepath.Join(dir, "out.png")
	writeTestPNG(t, src, 128, 128)

	payload := []byte("dct payload")
	opt := Options{Algorithm: "dct", Quality: 8, Seed: []byte("carrier-dct-seed")}
	c := Get(KindImage)
	if err := c.Embed(src, out, payload, opt); err != nil {
		t.Fatalf("DCT Embed: %v", err)
	}
	got, err := c.Extract(out, opt)
	if err != nil {
		t.Fatalf("DCT Extract: %v", err)
	}
	if !bytes.HasPrefix(got, payload) {
		t.Fatalf("DCT payload 不一致: %q", got[:min(len(got), len(payload))])
	}
}

func TestImageCarrierCapacity(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writeTestPNG(t, src, 64, 64) // 64*64*3 = 12288 bits = 1536 bytes (depth1)

	c := Get(KindImage)
	cap, err := c.Capacity(src, testOpt("cap-seed"))
	if err != nil {
		t.Fatal(err)
	}
	if cap != 64*64*3/8 {
		t.Fatalf("容量期望 %d, 实际 %d", 64*64*3/8, cap)
	}
	ok, err := c.HasCapacity(src, cap, testOpt("cap-seed"))
	if err != nil || !ok {
		t.Fatalf("HasCapacity: %v %v", ok, err)
	}
	ok, _ = c.HasCapacity(src, cap+1, testOpt("cap-seed"))
	if ok {
		t.Fatal("超出容量应返回 false")
	}
}

func TestImageCarrierTooLarge(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "small.png")
	out := filepath.Join(dir, "out.png")
	writeTestPNG(t, src, 4, 4) // 4*4*3 = 48 bits = 6 bytes
	c := Get(KindImage)
	payload := bytes.Repeat([]byte("A"), 100)
	if err := c.Embed(src, out, payload, testOpt("small-seed")); err == nil {
		t.Fatal("容量不足应报错")
	}
}

// ---------------------------------------------------------------------------
// 尾部容器
// ---------------------------------------------------------------------------

func TestTailCarrierRoundTrip(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("StegGo tail container payload with 中文")

	cases := []struct {
		name string
		ext  string
		head []byte
	}{
		{"wav", ".wav", []byte("RIFF\x24\x00\x00\x00WAVEfmt ")},
		{"flac", ".flac", []byte("fLaC\x00\x00\x00\x22")},
		{"pdf", ".pdf", []byte("%PDF-1.4\n1 0 obj\n%%EOF\n")},
		{"mp4", ".mp4", []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := filepath.Join(dir, "src"+c.ext)
			out := filepath.Join(dir, "out"+c.ext)
			writeTestBytes(t, src, append(c.head, bytes.Repeat([]byte{0xAA}, 64)...))
			carrier := Get(KindAudio)
			if c.ext == ".pdf" {
				carrier = Get(KindPDF)
			}
			if c.ext == ".mp4" {
				carrier = Get(KindVideo)
			}
			opt := testOpt("tail-" + c.name)
			if err := carrier.Embed(src, out, payload, opt); err != nil {
				t.Fatalf("Embed: %v", err)
			}
			got, err := carrier.Extract(out, opt)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload 不一致: %q != %q", got, payload)
			}
		})
	}
}

func TestTailCarrierNoPayload(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.wav")
	writeTestBytes(t, src, []byte("RIFF\x24\x00\x00\x00WAVEfmt "))
	c := Get(KindAudio)
	if _, err := c.Extract(src, testOpt("x")); err == nil {
		t.Fatal("无载荷文件应返回 ErrNoPayload")
	}
}

// ---------------------------------------------------------------------------
// 零宽文本载体
// ---------------------------------------------------------------------------

func TestTextCarrierRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "note.txt")
	out := filepath.Join(dir, "stego.txt")
	writeTestBytes(t, src, []byte("这是一段普通文本内容，用于测试零宽字符隐写。\nsecond line\n"))

	payload := []byte("zw-payload-零宽字符测试")
	c := Get(KindText)
	opt := testOpt("text-seed")
	if err := c.Embed(src, out, payload, opt); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	got, err := c.Extract(out, opt)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload 不一致: %q != %q", got, payload)
	}
	// 可见文本应保留
	visible := bytes.ReplaceAll(mustRead(t, out), []byte("\u200b"), nil)
	visible = bytes.ReplaceAll(visible, []byte("\u200d"), nil)
	if !bytes.Contains(visible, []byte("普通文本")) {
		t.Fatal("嵌入后可见文本被破坏")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Polyglot
// ---------------------------------------------------------------------------

func TestPolyglot(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	zip := filepath.Join(dir, "data.zip")
	out := filepath.Join(dir, "poly.pdf")
	writeTestBytes(t, pdf, []byte("%PDF-1.4\n...pdf body...\n%%EOF\n"))
	writeTestBytes(t, zip, []byte{'P', 'K', 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 'z', 'i', 'p', 'd', 'a', 't', 'a'})

	if err := BuildPolyglot([]string{pdf, zip}, out); err != nil {
		t.Fatal(err)
	}
	formats, err := DetectPolyglot(out)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFormat(formats, FmtPDF) || !containsFormat(formats, FmtZIP) {
		t.Fatalf("Polyglot 应检测到 pdf+zip, 实际 %v", formats)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("输出文件不存在")
	}
}

func containsFormat(list []PolyglotFormat, want PolyglotFormat) bool {
	for _, f := range list {
		if f == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 套娃
// ---------------------------------------------------------------------------

func TestNestedEmbedExtract(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner.png")
	outer := filepath.Join(dir, "outer.png")
	innerOut := filepath.Join(dir, "inner_out.png")
	outerOut := filepath.Join(dir, "outer_out.png")
	writeTestPNG(t, inner, 128, 128)
	writeTestPNG(t, outer, 512, 512) // 外层需容纳内层输出文件的完整字节

	payload := []byte("nested payload inside 套娃")
	seed := []byte("nested-seed")
	layers := []Layer{
		{CarrierPath: inner, OutPath: innerOut, Opt: Options{Algorithm: "lsb", BitDepth: 1, Seed: seed}},
		{CarrierPath: outer, OutPath: outerOut, Opt: Options{Algorithm: "lsb", BitDepth: 2, Seed: seed}},
	}
	outs, err := NestedEmbed(layers, payload)
	if err != nil {
		t.Fatalf("NestedEmbed: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("输出层数错误: %v", outs)
	}
	// 提取参数按 外层→内层 顺序传入（与嵌入 layers 相反）
	got, err := NestedExtract(outerOut, 2, layers[1].Opt, layers[0].Opt)
	if err != nil {
		t.Fatalf("NestedExtract: %v", err)
	}
	// 图像载体提取返回完整容量流（payload 后带噪声填充），载荷头部由上层解析。
	if !bytes.HasPrefix(got, payload) {
		t.Fatalf("套娃载荷不一致: %q", got[:min(len(got), len(payload))])
	}
}

func TestNestedEmbedSingleLayer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "single.png")
	out := filepath.Join(dir, "single_out.png")
	writeTestPNG(t, src, 64, 64)
	payload := []byte("single-layer payload")
	seed := []byte("single-seed")
	outs, err := NestedEmbed([]Layer{{CarrierPath: src, OutPath: out, Opt: Options{Seed: seed}}}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 {
		t.Fatalf("单层输出错误: %v", outs)
	}
	got, err := NestedExtract(out, 1, Options{Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, payload) {
		t.Fatalf("单层载荷不一致: %q", got[:min(len(got), len(payload))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 确保算法包依赖正确编译（引用 Normalize 导出）。
var _ = algorithm.Get
