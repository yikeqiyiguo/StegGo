package carrier

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestTransparentPNGRoundtrip 验证半透明/透明 PNG 载体在 Encode-Decode 后
// 像素值保持不变，避免 toNRGBA 的 alpha 预乘失真导致隐写数据丢失。
func TestTransparentPNGRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rgba.png")

	// 构造一个含半透明和全透明像素的 NRGBA 图像
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 100, G: 150, B: 200, A: uint8((x + y) % 256)}
			if x > 16 && y > 16 {
				c.A = 0
			}
			img.SetNRGBA(x, y, c)
		}
	}

	// 保存为 PNG
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// 用 LoadImage -> SaveImage 走一遍，像素应完全一致
	loaded, err := LoadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.png")
	if err := SaveImage(loaded, out); err != nil {
		t.Fatal(err)
	}
	loaded2, err := LoadImage(out)
	if err != nil {
		t.Fatal(err)
	}

	b := loaded.Bounds()
	diff := 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c1 := loaded.NRGBAAt(x, y)
			c2 := loaded2.NRGBAAt(x, y)
			if c1 != c2 {
				diff++
				if diff <= 3 {
					t.Logf("像素 (%d,%d) 不一致: %v -> %v", x, y, c1, c2)
				}
			}
		}
	}
	if diff > 0 {
		t.Errorf("LoadImage/SaveImage 后 %d 像素不一致", diff)
	}
}

// TestTransparentCarrierEmbedExtract 验证半透明 PNG 可作为隐写载体正常往返。
func TestTransparentCarrierEmbedExtract(t *testing.T) {
	dir := t.TempDir()
	carrier := filepath.Join(dir, "carrier.png")

	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			c := color.NRGBA{R: uint8(x), G: uint8(y), B: 128, A: uint8((x + y) / 2)}
			img.SetNRGBA(x, y, c)
		}
	}
	f, _ := os.Create(carrier)
	png.Encode(f, img)
	f.Close()

	payload := append([]byte("STEGGO3A"), bytes.Repeat([]byte{0}, 32)...)
	out := filepath.Join(dir, "out.png")
	c, err := ForPath(carrier)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Embed(carrier, out, payload, Options{
		Algorithm: "lsb",
		BitDepth:  1,
		Seed:      []byte("seed"),
	}); err != nil {
		t.Fatalf("嵌入失败: %v", err)
	}

	stream, err := c.Extract(out, Options{
		Algorithm: "lsb",
		BitDepth:  1,
		Seed:      []byte("seed"),
	})
	if err != nil {
		t.Fatalf("提取失败: %v", err)
	}
	if !bytes.HasPrefix(stream, []byte("STEGGO3A")) {
		end := 16
		if len(stream) < end {
			end = len(stream)
		}
		t.Errorf("提取出的位流前缀不是 MagicV3: %x", stream[:end])
	}
}
