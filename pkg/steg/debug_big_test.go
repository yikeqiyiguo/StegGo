package steg

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"steggo/pkg/carrier"
	"steggo/pkg/crypto"
)

func TestDebugBigRoundtrip(t *testing.T) {
	carrierPath := "../../testdata/carrier.png"
	pass := []byte("testpass123")
	depth := 1

	big := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(big, bytes.Repeat([]byte("Hello StegGo World 0123456789\n"), 200), 0644); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(big)

	// 1) 载荷构造：检查 ZIP 与密文
	payload, h, err := BuildSecretPayload(big, pass, Options{Password: pass, BitDepth: depth})
	if err != nil {
		t.Fatal(err)
	}
	defer crypto.Wipe(payload)
	fmt.Printf("payloadLen=%d flags=%d zip=%v\n", len(payload), h.Flags, h.Flags&flagZIP != 0)

	// 不经过图像，直接解析载荷（验证 ZIP 本身没问题）
	plain1, hh, err := ParseSecretPayload(payload, pass)
	if err != nil {
		t.Fatalf("direct parse failed: %v", err)
	}
	WipeHeader(hh)
	fmt.Printf("direct parse ok, len=%d equal=%v\n", len(plain1), bytes.Equal(plain1, want))

	// 2) 图像往返
	img, err := carrier.LoadImage(carrierPath)
	if err != nil {
		t.Fatal(err)
	}
	capacity := CapLSBBytes(img, depth)
	fmt.Printf("capacity=%d need=%d fit=%v\n", capacity, (len(payload)+8+7)/8*8, (len(payload)+8+7)/8*8 <= capacity)

	out := filepath.Join(t.TempDir(), "out.png")
	if _, err := EmbedImage(carrierPath, out, big, Options{Password: pass, BitDepth: depth}); err != nil {
		t.Fatal(err)
	}
	img2, err := carrier.LoadImage(out)
	if err != nil {
		t.Fatal(err)
	}
	seed := SeedFromPassword(pass)
	defer crypto.Wipe(seed)
	stream, err := ExtractLSB(img2, seed, depth)
	if err != nil {
		t.Fatal(err)
	}
	gotBits := ByteToBits(payload)
	fmt.Printf("streamLen=%d payloadBits=%d\n", len(stream), len(gotBits))
	// 比较前 len(gotBits) 位
	same := true
	firstDiff := -1
	n := len(gotBits)
	if n > len(stream) {
		same = false
		firstDiff = n
	}
	for i := 0; i < n && i < len(stream); i++ {
		if stream[i] != gotBits[i] {
			same = false
			firstDiff = i
			break
		}
	}
	fmt.Printf("first %d bits equal=%v firstDiff=%d (%.1f%%)\n", n, same, firstDiff, float64(firstDiff)/float64(n)*100)

	// 3) 用提取的位流构造 payload 并解析
	back := BitsToBytes(stream)
	if len(back) >= 8 && string(back[:8]) == string(MagicV2) {
		plain2, h2, err := ParseSecretPayload(back, pass)
		if err != nil {
			fmt.Printf("image roundtrip parse error: %v\n", err)
		} else {
			fmt.Printf("image roundtrip ok, len=%d equal=%v\n", len(plain2), bytes.Equal(plain2, want))
			WipeHeader(h2)
		}
	} else {
		fmt.Printf("magic mismatch: back[0:8]=%q\n", back[:8])
	}

	// 4) 完整提取
	extDir := t.TempDir()
	res, err := ExtractImage(out, extDir, pass)
	if err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(extDir, res.Name))
	fmt.Printf("ExtractImage ok name=%s size=%d equal=%v\n", res.Name, len(got), bytes.Equal(got, want))
}
