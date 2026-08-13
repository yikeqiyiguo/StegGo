package steg

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReproEmbedExtract(t *testing.T) {
	carrier := "../../testdata/carrier.png"
	secret := "../../testdata/secret.txt"
	pass := []byte("testpass123")

	big := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(big, bytes.Repeat([]byte("Hello StegGo World 0123456789\n"), 200), 0644); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(big)

	secrets := []string{secret, big}
	for _, sc := range secrets {
		for depth := 1; depth <= 4; depth++ {
			out := filepath.Join(t.TempDir(), "out.png")
			if _, err := EmbedImage(carrier, out, sc, Options{Password: pass, BitDepth: depth}); err != nil {
				t.Fatalf("embed %s depth=%d: %v", sc, depth, err)
			}
			outDir := filepath.Join(t.TempDir(), "x")
			res, err := ExtractImage(out, outDir, pass)
			if err != nil {
				t.Fatalf("extract %s depth=%d: %v", sc, depth, err)
			}
			t.Logf("ok %s depth=%d name=%s size=%d", filepath.Base(sc), depth, res.Name, res.RawSize)
			got, _ := os.ReadFile(filepath.Join(outDir, res.Name))
			if sc == big && !bytes.Equal(got, want) {
				t.Fatalf("extract %s depth=%d: 内容不一致 len got=%d want=%d", filepath.Base(sc), depth, len(got), len(want))
			}
		}
	}
}
