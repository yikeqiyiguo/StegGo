package steg

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// newTestImage 生成指定尺寸的测试图像（带随机噪点以贴近真实照片）。
func newTestImage(w, h int, seed int64) *image.NRGBA {
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

func TestByteBitsRoundTrip(t *testing.T) {
	data := []byte("StegGo Bit Round Trip \x00\x01\xFF\xFE")
	bits := ByteToBits(data)
	if len(bits) != len(data)*8 {
		t.Fatalf("位数组长度应为 %d, 实际 %d", len(data)*8, len(bits))
	}
	out := BitsToBytes(bits)
	if !bytes.Equal(out, data) {
		t.Fatal("位转换往返不一致")
	}
}

func TestEmbedExtractRoundTrip(t *testing.T) {
	secret := bytes.Repeat([]byte("Anti-detection LSB payload "), 20)
	bits := ByteToBits(secret)
	seed := []byte("roundtrip-seed")

	for _, depth := range []int{1, 2, 3, 4} {
		img := newTestImage(96, 96, 42)
		capBytes := CapLSBBytes(img, depth)
		if capBytes < len(secret) {
			t.Fatalf("位深度 %d 容量不足: %d < %d", depth, capBytes, len(secret))
		}
		if err := EmbedLSB(img, bits, seed, depth); err != nil {
			t.Fatalf("位深度 %d EmbedLSB: %v", depth, err)
		}
		stream, err := ExtractLSB(img, seed, depth)
		if err != nil {
			t.Fatalf("位深度 %d ExtractLSB: %v", depth, err)
		}
		// 数据位应完整保留在流头部
		if !bytes.Equal(stream[:len(bits)], bits) {
			t.Fatalf("位深度 %d 数据位不一致", depth)
		}
		// 数据位之后的噪声区也应有内容（不应全是 0）
		noise := stream[len(bits):]
		if !hasAnyBit(noise) {
			t.Fatalf("位深度 %d 噪声区应包含随机位", depth)
		}
	}
}

func TestEmbedLSBChangesImage(t *testing.T) {
	img := newTestImage(32, 32, 7)
	orig := image.NewNRGBA(img.Bounds())
	copy(orig.Pix, img.Pix)
	if err := EmbedLSB(img, ByteToBits([]byte("changes")), []byte("s"), 2); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(orig.Pix, img.Pix) {
		t.Fatal("嵌入后图像像素不应完全不变")
	}
}

func TestEmbedInvalidDepth(t *testing.T) {
	img := newTestImage(16, 16, 1)
	if err := EmbedLSB(img, []byte{0}, []byte("s"), 0); err == nil {
		t.Fatal("位深度 0 应报错")
	}
	if err := EmbedLSB(img, []byte{0}, []byte("s"), 5); err == nil {
		t.Fatal("位深度 5 应报错")
	}
}

func TestEmbedNilImage(t *testing.T) {
	if err := EmbedLSB(nil, []byte{0}, []byte("s"), 2); err == nil {
		t.Fatal("空图像应报错")
	}
}

func TestCapLSBBytes(t *testing.T) {
	img := newTestImage(100, 100, 3)
	// 100*100*3*1/8 = 3750
	if got := CapLSBBytes(img, 1); got != 100*100*3/8 {
		t.Fatalf("位深度 1 容量错误: %d", got)
	}
	if got := CapLSBBytes(img, 4); got != 100*100*3*4/8 {
		t.Fatalf("位深度 4 容量错误: %d", got)
	}
}

func hasAnyBit(bits []byte) bool {
	for _, b := range bits {
		if b != 0 {
			return true
		}
	}
	return false
}
