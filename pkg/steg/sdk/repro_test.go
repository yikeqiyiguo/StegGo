package sdk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestGUIAllAlgorithmsRoundTrip 模拟 GUI 完整流程（嵌入 → 自动扫描提取），
// 覆盖全部内置算法，验证提取内容与原文一致。
func TestGUIAllAlgorithmsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	carrier := filepath.Join(dir, "carrier.png")
	secret := filepath.Join(dir, "secret.bin")

	in, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "carrier.png"))
	if err != nil {
		t.Fatalf("read carrier: %v", err)
	}
	if err := os.WriteFile(carrier, in, 0o644); err != nil {
		t.Fatal(err)
	}
	// 秘密内容：可读文本 + 随机字节，保证非 ASCII 内容也能完整往返
	payload := make([]byte, 0, 2048)
	payload = append(payload, []byte("hello steggo 你好 secret payload for repro ")...)
	for i := 0; i < 1024; i++ {
		payload = append(payload, byte(i%251))
	}
	if err := os.WriteFile(secret, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		algo string
		bits int
	}{
		{"lsb", 2}, {"lsb", 1}, {"lsb", 4},
		{"dct", 2},
		{"dwt", 2},
		{"hugo", 2},
		{"wow", 2},
		{"uniward", 2},
	}
	for _, tc := range cases {
		t.Run(tc.algo, func(t *testing.T) {
			out := filepath.Join(dir, tc.algo+".png")
			_, err := Embed(Options{
				CarrierPath: carrier,
				SecretPath:  secret,
				OutputPath:  out,
				Password:    []byte("test-password-123"),
				Algorithm:   tc.algo,
				BitDepth:    tc.bits,
			})
			if err != nil {
				t.Fatalf("embed 失败: %v", err)
			}
			exDir := filepath.Join(dir, tc.algo+"_out")
			res, err := Extract(Options{
				CarrierPath: out,
				OutputPath:  exDir,
				Password:    []byte("test-password-123"),
			})
			if err != nil {
				t.Fatalf("extract 失败: %v", err)
			}
			if res.Size != int64(len(payload)) {
				t.Fatalf("提取尺寸不一致: got %d want %d", res.Size, len(payload))
			}
			got, err := os.ReadFile(filepath.Join(exDir, res.Name))
			if err != nil {
				t.Fatalf("read extracted: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("提取内容不一致: got %d bytes want %d bytes", len(got), len(payload))
			}
			t.Logf("提取成功: name=%s size=%d algo=%s bits=%d", res.Name, res.Size, res.Algorithm, res.BitDepth)
		})
	}
}

// TestExtractWrongPassword 验证错误密码时提取必须失败（防止扫描误报）。
func TestExtractWrongPassword(t *testing.T) {
	dir := t.TempDir()
	carrier := filepath.Join(dir, "carrier.png")
	secret := filepath.Join(dir, "secret.txt")

	in, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "carrier.png"))
	if err != nil {
		t.Fatalf("read carrier: %v", err)
	}
	if err := os.WriteFile(carrier, in, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret for wrong password test"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "steg.png")
	if _, err := Embed(Options{
		CarrierPath: carrier,
		SecretPath:  secret,
		OutputPath:  out,
		Password:    []byte("correct-password"),
		Algorithm:   "lsb",
		BitDepth:    2,
	}); err != nil {
		t.Fatalf("embed: %v", err)
	}
	// 用错误密码提取必须失败（DCT 魔数命中也必须在解密阶段被拦截）
	if _, err := Extract(Options{
		CarrierPath: out,
		OutputPath:  filepath.Join(dir, "out_wrong"),
		Password:    []byte("wrong-password"),
	}); err == nil {
		t.Fatal("错误密码提取应当失败")
	}
}
