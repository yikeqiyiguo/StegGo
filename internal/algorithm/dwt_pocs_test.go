package algorithm

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestDWTPOCSRoundtrip 验证 DWT POCS 全链路（Embed→PNG→Extract）零翻转。
func TestDWTPOCSRoundtrip(t *testing.T) {
	for _, levels := range []int{1, 2, 3} {
		img := makeTestImage(64, 64)
		opt := Options{Levels: levels, Seed: []byte("dwt-pocs-seed")}
		a := NewDWT()
		cap := a.Capacity(img, opt)
		if cap < 200 {
			t.Fatalf("levels=%d: capacity too small: %d", levels, cap)
		}
		n := cap / 4
		bits := makeDataBits(n)

		work := cloneImg(img)
		if err := a.Embed(work, bits, opt); err != nil {
			t.Fatalf("levels=%d: embed: %v", levels, err)
		}
		// PNG 往返模拟真实链路（t.TempDir 自动清理，不污染源码目录）
		tmpPNG := filepath.Join(t.TempDir(), "work.png")
		writeDWTTestPNG(t, work, tmpPNG)
		work2 := loadDWTTestPNG(t, tmpPNG)

		got, err := a.Extract(work2, opt)
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

// TestDWTPOCSNonMultipleOf8 用非 8 倍数尺寸（且非 2 倍数）验证 DWT 全链路。
func TestDWTPOCSNonMultipleOf8(t *testing.T) {
	img := makeTestImage(300, 300)
	opt := Options{Levels: 2, Seed: []byte("non-multiple-8")}
	a := NewDWT()
	// 使用与真实 payload 相近长度
	payload := make([]byte, 77)
	for i := range payload {
		payload[i] = byte(i*3 + 1)
	}
	bits := ByteToBits(payload)
	work := cloneImg(img)
	if err := a.Embed(work, bits, opt); err != nil {
		t.Fatal(err)
	}
	tmpPNG := filepath.Join(t.TempDir(), "work.png")
	writeDWTTestPNG(t, work, tmpPNG)
	work2 := loadDWTTestPNG(t, tmpPNG)
	got, err := a.Extract(work2, opt)
	if err != nil {
		t.Fatal(err)
	}
	for i := range bits {
		if got[i] != bits[i] {
			t.Fatalf("bit %d 翻转", i)
		}
	}
}

// TestDWTPOCSRealCarrier 用真实载体 PNG 验证 DWT 全链路零翻转。
func TestDWTPOCSRealCarrier(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Skipf("跳过: %v", err)
	}
	opt := Options{Levels: 2, Seed: []byte("real-carrier")}
	a := NewDWT()
	// 模拟真实载荷（77B）
	payload := make([]byte, 77)
	for i := range payload {
		payload[i] = byte(i + 1)
	}
	bits := ByteToBits(payload)
	work := cloneImg(img)
	if err := a.Embed(work, bits, opt); err != nil {
		t.Fatal(err)
	}
	tmpPNG := filepath.Join(t.TempDir(), "work.png")
	writeDWTTestPNG(t, work, tmpPNG)
	work2 := loadDWTTestPNG(t, tmpPNG)
	got, err := a.Extract(work2, opt)
	if err != nil {
		t.Fatal(err)
	}
	for i := range bits {
		if got[i] != bits[i] {
			t.Fatalf("bit %d 翻转", i)
		}
	}
	psnr := PSNR(img, work)
	t.Logf("real carrier: PSNR=%.1f dB", psnr)
}

func writeDWTTestPNG(t *testing.T, img *image.NRGBA, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func loadDWTTestPNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return dst
}
