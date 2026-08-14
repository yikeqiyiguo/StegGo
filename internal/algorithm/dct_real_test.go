package algorithm

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// loadTestCarrier 加载 testdata 真实 PNG 载体。
func loadTestCarrier() (*image.NRGBA, error) {
	f, err := os.Open("../../testdata/carrier.png")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	img := image.NewNRGBA(src.Bounds())
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return img, nil
}

func cloneImg(img *image.NRGBA) *image.NRGBA {
	cp := image.NewNRGBA(img.Rect)
	copy(cp.Pix, img.Pix)
	return cp
}

func TestDCTRoundtripRealCarrier(t *testing.T) {
	img, err := loadTestCarrier()
	if err != nil {
		t.Fatal(err)
	}
	a := NewDCT()
	seed := []byte("test-seed-dct-real")
	secret := bytes.Repeat([]byte("StegGo DCT payload roundtrip test!!"), 20)
	for _, q := range []int{8, 16} {
		opt := Options{Quality: q, Seed: seed}
		cap := a.Capacity(img, opt)
		if cap < len(secret)*8 {
			t.Fatalf("q=%d capacity %d < need %d", q, cap, len(secret)*8)
		}
		bits := ByteToBits(secret)
		orig := cloneImg(img)
		if err := a.Embed(img, bits, opt); err != nil {
			t.Fatalf("q=%d embed: %v", q, err)
		}
		gotBits, err := a.Extract(img, Options{Quality: q, Seed: seed})
		if err != nil {
			t.Fatalf("q=%d extract: %v", q, err)
		}
		got := BitsToBytes(gotBits[:len(bits)])
		if !bytes.Equal(got, secret) {
			diff := -1
			for i := range secret {
				if got[i] != secret[i] {
					diff = i
					break
				}
			}
			t.Fatalf("q=%d roundtrip mismatch at byte %d\nwant: %q\ngot:  %q", q, diff, secret[:32], got[:32])
		}
		// 图像不应与原图完全相同（嵌入生效）
		if bytes.Equal(img.Pix, orig.Pix) {
			t.Fatalf("q=%d image unchanged, embed no-op", q)
		}
	}
}
