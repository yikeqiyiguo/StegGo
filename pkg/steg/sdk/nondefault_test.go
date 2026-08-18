package sdk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestNonDefaultParamsScanExtract 验证"CLI 以自定义参数嵌入 → GUI 自动扫描提取"：
// 旧版硬编码扫描矩阵只试 DCT q=8/16，自定义参数嵌入后 GUI 提取必然失败。
// 本测试确认新扫描矩阵能覆盖自定义参数。
func TestNonDefaultParamsScanExtract(t *testing.T) {
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
	if err := os.WriteFile(secret, []byte("custom param embed payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		algo    string
		quality int
		levels  int
		bits    int
	}{
		{algo: "dct", quality: 6},
		{algo: "dct", quality: 4},
		{algo: "dwt", levels: 1},
		{algo: "lsb", bits: 3},
	}
	for _, tc := range cases {
		out := filepath.Join(dir, tc.algo+".png")
		if _, err := Embed(Options{
			CarrierPath: carrier,
			SecretPath:  secret,
			OutputPath:  out,
			Password:    []byte("pass"),
			Algorithm:   tc.algo,
			Quality:     tc.quality,
			Levels:      tc.levels,
			BitDepth:    tc.bits,
		}); err != nil {
			t.Fatalf("[%s q=%d l=%d b=%d] embed: %v", tc.algo, tc.quality, tc.levels, tc.bits, err)
		}
		exDir := filepath.Join(dir, "out")
		res, err := Extract(Options{
			CarrierPath: out,
			OutputPath:  exDir,
			Password:    []byte("pass"),
		})
		if err != nil {
			t.Fatalf("[%s q=%d l=%d b=%d] 自动扫描提取失败: %v", tc.algo, tc.quality, tc.levels, tc.bits, err)
		}
		got, err := os.ReadFile(filepath.Join(exDir, res.Name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, []byte("custom param embed payload")) {
			t.Fatalf("[%s] 内容不一致", tc.algo)
		}
		t.Logf("[%s q=%d l=%d b=%d] 提取成功", tc.algo, tc.quality, tc.levels, tc.bits)
	}
}
