package carrier

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestDetectKind(t *testing.T) {
	cases := map[string]Kind{
		"a.png":      KindImage,
		"b.bmp":      KindImage,
		"c.tif":      KindImage,
		"d.tiff":     KindImage,
		"e.wav":      KindAudio,
		"f.pdf":      KindPDF,
		"g.txt":      KindText,
		"h.md":       KindText,
		"i.markdown": KindText,
		"j.mp4":      KindVideo,
		"k.mkv":      KindVideo,
		"l.webm":     KindVideo,
		"m.unknown":  KindUnknown,
		"n.jpg":      KindUnknown,
		"o.jpeg":     KindUnknown,
		"p.mp3":      KindUnknown,
	}
	for name, want := range cases {
		got, err := DetectKind(name)
		if want == KindUnknown {
			if err == nil {
				t.Errorf("%s 应当返回错误", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%s: 期望 %v, 实际 %v", name, want, got)
		}
	}
}

func TestDetectKindRejectsLossy(t *testing.T) {
	for _, name := range []string{"a.jpg", "b.JPG", "c.jpeg"} {
		if _, err := DetectKind(name); err == nil {
			t.Errorf("%s 有损格式应被拦截", name)
		}
	}
}

func TestIsSupported(t *testing.T) {
	if !IsSupported("x.png") {
		t.Fatal("png 应受支持")
	}
	if IsSupported("x.jpg") {
		t.Fatal("jpg 不应受支持")
	}
	if IsSupported("x.exe") {
		t.Fatal("exe 不应受支持")
	}
}

func TestLoadImagePNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	writeTestPNG(t, path)

	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 8 || b.Dy() != 8 {
		t.Fatalf("尺寸错误: %v", b)
	}
}

func TestLoadImageRejectsJPEGHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.jpg")
	// 伪造 JPEG 文件头 FF D8 FF E0，即使扩展名是 jpg 也会被文件头拦截
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImage(path); err == nil {
		t.Fatal("JPEG 文件头应当被拦截")
	}
}

func TestSaveImageBMPRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.bmp")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	if err := SaveBMP(img, path); err != nil {
		t.Fatalf("SaveBMP: %v", err)
	}
	loaded, err := LoadImage(path)
	if err != nil {
		t.Fatalf("加载 BMP: %v", err)
	}
	if loaded.Bounds().Dx() != 4 {
		t.Fatal("BMP 尺寸不一致")
	}
}

func TestFileSizeAndInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sz.png")
	writeTestPNG(t, path)

	sz, err := FileSize(path)
	if err != nil || sz <= 0 {
		t.Fatalf("FileSize: %v", err)
	}
	info, err := GetImageInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 8 || info.Height != 8 || info.Channels != 3 {
		t.Fatalf("ImageInfo 错误: %+v", info)
	}
}
