package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const testCarrier = "../../testdata/carrier.png"
const testSecret = "../../testdata/secret.txt"

// TestWrapUnwrapECC 验证 RS 编码包装/解包往返及损坏修复。
func TestWrapUnwrapECC(t *testing.T) {
	data := bytes.Repeat([]byte("StegGo ECC 容错编码测试 payload 0123456789"), 20)
	wrapped, _, err := wrapECC(data, "high")
	if err != nil {
		t.Fatal(err)
	}
	if len(wrapped) <= len(data) {
		t.Fatal("ECC 编码应增加冗余")
	}
	got, lv, stats, ok, err := unwrapECC(wrapped)
	if err != nil || !ok {
		t.Fatalf("unwrap: ok=%v err=%v", ok, err)
	}
	if lv != "high" {
		t.Fatalf("等级 %s", lv)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("往返不一致")
	}
	if stats.Blocks == 0 {
		t.Fatal("块数应为正")
	}

	// 损坏部分字节（跳过 origLen 头，分散到各块，每块不超过冗余容量），应可修复
	damaged := append([]byte(nil), wrapped...)
	for i := 0; i < 12; i++ {
		pos := 14 + i*47 // 14=头部10+origLen4；间隔 47B 保证每 255B 块内损坏 < 16 符号
		if pos >= len(damaged) {
			break
		}
		damaged[pos] ^= 0x03
	}
	recovered, _, stats2, ok2, err := unwrapECC(damaged)
	if err != nil || !ok2 {
		t.Fatalf("损坏恢复: ok=%v err=%v", ok2, err)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatal("RS 纠错未能恢复损坏数据")
	}
	if stats2.CorrectedErrors == 0 {
		t.Fatal("应统计到纠正的符号数")
	}
}

// TestECCEmbedExtract 验证启用 ECC 的嵌入/提取往返。
func TestECCEmbedExtract(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "ecc.png")
	if _, err := New().Embed(Options{
		CarrierPath: testCarrier,
		SecretPath:  testSecret,
		OutputPath:  out,
		Password:    []byte("pw"),
		Algorithm:   "lsb",
		BitDepth:    2,
		ECC:         "high",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := New().Extract(Options{
		CarrierPath: out,
		OutputPath:  dir,
		Password:    []byte("pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ECCLevel != "high" {
		t.Fatalf("ECCLevel=%q", res.ECCLevel)
	}
	if res.ECCBlocks == 0 {
		t.Fatal("ECCBlocks 应为正")
	}
	got, err := os.ReadFile(filepath.Join(dir, res.Name))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("提取内容与原始秘密不一致")
	}
}

// TestECCRepairDamagedCarrier 验证载体局部损坏后 RS 纠错恢复提取。
func TestECCRepairDamagedCarrier(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "ecc.png")
	if _, err := New().Embed(Options{
		CarrierPath: testCarrier,
		SecretPath:  testSecret,
		OutputPath:  out,
		Password:    []byte("pw"),
		Algorithm:   "lsb",
		BitDepth:    2,
		ECC:         "medium",
	}); err != nil {
		t.Fatal(err)
	}

	// 篡改载体中 24 个像素的 LSB 位（模拟社交压缩/局部损坏）
	img, err := readPNG(out)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	for i := 0; i < 24; i++ {
		x := bounds.Min.X + i*7
		y := bounds.Min.Y + i*5
		if x >= bounds.Max.X || y >= bounds.Max.Y {
			continue
		}
		c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		img.Set(x, y, color.NRGBA{
			R: c.R ^ 0x03,
			G: c.G ^ 0x03,
			B: c.B ^ 0x03,
			A: c.A,
		})
	}
	damaged := filepath.Join(dir, "damaged.png")
	if err := writePNG(damaged, img); err != nil {
		t.Fatal(err)
	}

	res, err := New().Extract(Options{
		CarrierPath: damaged,
		OutputPath:  dir,
		Password:    []byte("pw"),
	})
	if err != nil {
		t.Fatalf("损坏载体提取失败（RS 应修复）: %v", err)
	}
	if res.ECCLevel != "medium" {
		t.Fatalf("ECCLevel=%q", res.ECCLevel)
	}
	got, err := os.ReadFile(filepath.Join(dir, res.Name))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("RS 修复后内容不一致")
	}
}

// readPNG 读取 PNG 图像（统一转为 NRGBA 以便像素级修改）。
func readPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba, nil
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, nil
}

// writePNG 写出 PNG 图像。
func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
