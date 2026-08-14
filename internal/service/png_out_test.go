package service

import (
	"path/filepath"
	"testing"
)

// TestPNGOutputRoundtrip 复现用户场景：GUI 嵌入时手动指定 .png 输出（非默认 .steg）。
func TestPNGOutputRoundtrip(t *testing.T) {
	carrierPath := filepath.Join("..", "..", "testdata", "carrier.png")
	secretPath := filepath.Join("..", "..", "testdata", "secret.txt")
	algos := []string{"lsb", "dct", "dwt", "hugo", "wow", "uniward"}
	dir := t.TempDir()

	for _, algo := range algos {
		t.Run(algo, func(t *testing.T) {
			out := filepath.Join(dir, "out_"+algo+".png")
			_, err := New().Embed(Options{
				CarrierPath: carrierPath,
				SecretPath:  secretPath,
				OutputPath:  out,
				Password:    []byte("testpass"),
				Algorithm:   algo,
				BitDepth:    2,
			})
			if err != nil {
				t.Fatalf("嵌入失败: %v", err)
			}
			_, err = New().Extract(Options{
				CarrierPath: out,
				OutputPath:  filepath.Join(dir, "ex_"+algo),
				Password:    []byte("testpass"),
			})
			if err != nil {
				t.Fatalf("提取失败: %v", err)
			}
			t.Logf("%s .png 输出 → 提取成功 ✓", algo)
		})
	}
}
