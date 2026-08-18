package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"steggo/internal/carrier"
	v1steg "steggo/pkg/steg"
)

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

func makeTestImage(w, h int, seed int64) *image.NRGBA {
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

func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := carrier.SaveImage(makeTestImage(w, h, 7), path); err != nil {
		t.Fatal(err)
	}
}

func writeSecret(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func newTestService() *Service { return New() }

func baseOpt(carrierPath, secretPath, outPath string, pass []byte) Options {
	return Options{
		CarrierPath: carrierPath,
		SecretPath:  secretPath,
		OutputPath:  outPath,
		Password:    pass,
	}
}

// ---------------------------------------------------------------------------
// 核心嵌入/提取
// ---------------------------------------------------------------------------

func TestServiceEmbedExtractLSB(t *testing.T) {
	dir := t.TempDir()
	carrierPath := filepath.Join(dir, "carrier.png")
	outCarrier := filepath.Join(dir, "out.png")
	secretPath := filepath.Join(dir, "secret.txt")
	extractDir := filepath.Join(dir, "extracted")
	writeTestPNG(t, carrierPath, 128, 128)
	writeSecret(t, secretPath, "StegGo V2.0 service LSB 测试数据 你好世界")

	s := newTestService()
	res, err := s.Embed(baseOpt(carrierPath, secretPath, outCarrier, []byte("pass123")))
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if res.Name != "secret.txt" {
		t.Fatalf("Name 期望 secret.txt, 实际 %q", res.Name)
	}

	eres, err := s.Extract(baseOpt(outCarrier, "", extractDir, []byte("pass123")))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if eres.V1Compat {
		t.Fatal("V2 载体不应走 V1 兼容路径")
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(secretPath)
	if !bytes.Equal(got, want) {
		t.Fatalf("提取内容不一致: %q != %q", got, want)
	}
}

func TestServiceEmbedExtractAlgorithms(t *testing.T) {
	cases := []struct {
		name string
		opt  Options
	}{
		{"lsb-depth2", Options{Algorithm: "lsb", BitDepth: 2}},
		{"dct", Options{Algorithm: "dct", Quality: 8}},
		{"dwt", Options{Algorithm: "dwt", Levels: 2}},
		{"hugo", Options{Algorithm: "hugo", CostStyle: "hill"}},
		{"wow", Options{Algorithm: "wow", CostStyle: "wow"}},
		{"uniward", Options{Algorithm: "uniward", CostStyle: "uniward"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			carrierPath := filepath.Join(dir, "carrier.png")
			outCarrier := filepath.Join(dir, "out.png")
			secretPath := filepath.Join(dir, "secret.bin")
			extractDir := filepath.Join(dir, "x")
			writeTestPNG(t, carrierPath, 256, 256)
			writeSecret(t, secretPath, strings.Repeat("A", 64))

			s := newTestService()
			opt := baseOpt(carrierPath, secretPath, outCarrier, []byte("p"))
			opt.Algorithm = c.opt.Algorithm
			opt.BitDepth = c.opt.BitDepth
			opt.Quality = c.opt.Quality
			opt.Levels = c.opt.Levels
			opt.CostStyle = c.opt.CostStyle
			if _, err := s.Embed(opt); err != nil {
				t.Fatalf("Embed: %v", err)
			}
			// 提取时不指定算法，走自动扫描
			if _, err := s.Extract(baseOpt(outCarrier, "", extractDir, []byte("p"))); err != nil {
				t.Fatalf("Extract: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(extractDir, "secret.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bytes.Repeat([]byte("A"), 64)) {
				t.Fatalf("提取内容不一致")
			}
		})
	}
}

func TestServiceEmbedExtractTailCarrier(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "sound.wav")
	outWav := filepath.Join(dir, "out.wav")
	secretPath := filepath.Join(dir, "doc.txt")
	extractDir := filepath.Join(dir, "x")
	writeSecret(t, wavPath, "RIFF\x24\x00\x00\x00WAVEfmt  data\xAA\xBB")
	writeSecret(t, secretPath, "tail carrier 测试")

	s := newTestService()
	if _, err := s.Embed(baseOpt(wavPath, secretPath, outWav, []byte("p"))); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if _, err := s.Extract(baseOpt(outWav, "", extractDir, []byte("p"))); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "tail carrier 测试" {
		t.Fatalf("内容不一致: %q", got)
	}
}

func TestServiceDeniable(t *testing.T) {
	dir := t.TempDir()
	carrierPath := filepath.Join(dir, "c.png")
	outCarrier := filepath.Join(dir, "out.png")
	realPath := filepath.Join(dir, "real.txt")
	fakePath := filepath.Join(dir, "fake.txt")
	writeTestPNG(t, carrierPath, 128, 128)
	writeSecret(t, realPath, "真实秘密文件内容")
	writeSecret(t, fakePath, "诱饵文件内容（假的）")

	s := newTestService()
	opt := baseOpt(carrierPath, realPath, outCarrier, []byte("real-pass"))
	opt.FakeFile = fakePath
	opt.FakePassword = []byte("fake-pass")
	res, err := s.Embed(opt)
	if err != nil {
		t.Fatalf("Embed deniable: %v", err)
	}
	if !res.Deniable {
		t.Fatal("应标记为可否认")
	}

	// 真实密码提取 → real 区
	extractDir1 := filepath.Join(dir, "r")
	if _, err := s.Extract(baseOpt(outCarrier, "", extractDir1, []byte("real-pass"))); err != nil {
		t.Fatalf("real extract: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(extractDir1, "real.txt"))
	if string(got) != "真实秘密文件内容" {
		t.Fatalf("真实内容不一致: %q", got)
	}

	// 诱饵密码提取 → fake 区（胁迫场景）
	extractDir2 := filepath.Join(dir, "f")
	if _, err := s.Extract(baseOpt(outCarrier, "", extractDir2, []byte("fake-pass"))); err != nil {
		t.Fatalf("fake extract: %v", err)
	}
	got2, _ := os.ReadFile(filepath.Join(extractDir2, "real.txt"))
	if string(got2) != "诱饵文件内容（假的）" {
		t.Fatalf("诱饵内容不一致: %q", got2)
	}
}

func TestServiceWrongPassword(t *testing.T) {
	dir := t.TempDir()
	carrierPath := filepath.Join(dir, "c.png")
	outCarrier := filepath.Join(dir, "out.png")
	secretPath := filepath.Join(dir, "s.txt")
	writeTestPNG(t, carrierPath, 128, 128)
	writeSecret(t, secretPath, "data")
	s := newTestService()
	if _, err := s.Embed(baseOpt(carrierPath, secretPath, outCarrier, []byte("right"))); err != nil {
		t.Fatal(err)
	}
	extractDir := filepath.Join(dir, "x")
	if _, err := s.Extract(baseOpt(outCarrier, "", extractDir, []byte("wrong"))); err == nil {
		t.Fatal("错误密码应提取失败")
	}
}

func TestServiceExtractFromJPEGAnchored(t *testing.T) {
	dir := t.TempDir()
	carrierPath := filepath.Join(dir, "carrier.png")
	outPNG := filepath.Join(dir, "out.png")
	secretPath := filepath.Join(dir, "s.txt")
	jpgPath := filepath.Join(dir, "out.jpg")
	extractDir := filepath.Join(dir, "x")
	writeTestPNG(t, carrierPath, 512, 512)
	writeSecret(t, secretPath, "JPEG提取测试")

	s := newTestService()
	opt := baseOpt(carrierPath, secretPath, outPNG, []byte("p"))
	opt.Algorithm = "anchored"
	if _, err := s.Embed(opt); err != nil {
		t.Fatalf("Embed anchored: %v", err)
	}

	// 模拟社交平台重压缩：PNG → JPEG q75。
	if err := pngToJPEG(outPNG, jpgPath, 75); err != nil {
		t.Fatal(err)
	}

	// 不指定算法，走自动扫描；JPEG 提取放行。
	if _, err := s.Extract(baseOpt(jpgPath, "", extractDir, []byte("p"))); err != nil {
		t.Fatalf("Extract from JPEG: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "s.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "JPEG提取测试" {
		t.Fatalf("内容不一致: %q", got)
	}
}

func pngToJPEG(pngPath, jpgPath string, q int) error {
	f, err := os.Open(pngPath)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	out, err := os.Create(jpgPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, img, &jpeg.Options{Quality: q})
}

// ---------------------------------------------------------------------------
// 批量
// ---------------------------------------------------------------------------

func TestServiceBatchEmbedExtract(t *testing.T) {
	dir := t.TempDir()
	inDir := filepath.Join(dir, "in")
	outDir := filepath.Join(dir, "out")
	secretPath := filepath.Join(dir, "msg.txt")
	if err := os.MkdirAll(inDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(inDir, "a.png"), 64, 64)
	writeTestPNG(t, filepath.Join(inDir, "b.png"), 96, 96)
	writeSecret(t, secretPath, "batch 批量测试内容")

	s := newTestService()
	res, err := s.BatchEmbed(BatchOptions{
		Options:   Options{SecretPath: secretPath, Password: []byte("p"), Algorithm: "lsb", BitDepth: 1},
		InputDir:  inDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("BatchEmbed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("应嵌入 2 个载体, 实际 %d", len(res))
	}
	ok, fail := batchSummary(res)
	if fail != 0 {
		t.Fatalf("存在失败项: ok=%d fail=%d", ok, fail)
	}

	// 批量提取
	res2, err := s.BatchExtract(BatchOptions{
		Options:   Options{Password: []byte("p")},
		InputDir:  outDir,
		OutputDir: filepath.Join(dir, "x"),
	})
	if err != nil {
		t.Fatalf("BatchExtract: %v", err)
	}
	ok2, fail2 := batchSummary(res2)
	if fail2 != 0 {
		t.Fatalf("提取失败项: ok=%d fail=%d", ok2, fail2)
	}
}

// ---------------------------------------------------------------------------
// Shamir 分权
// ---------------------------------------------------------------------------

func TestServiceShamirSplitRecover(t *testing.T) {
	dir := t.TempDir()
	var carriers []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, "c.png")
		writeTestPNG(t, p, 64, 64)
		carriers = append(carriers, p)
	}
	secretPath := filepath.Join(dir, "secret.txt")
	writeSecret(t, secretPath, "Shamir 分权测试秘密数据")

	s := newTestService()
	sopt := ShamirOptions{
		Options:      Options{SecretPath: secretPath, Password: []byte("p")},
		Total:        3,
		Threshold:    2,
		CarrierPaths: carriers,
		ShareDir:     filepath.Join(dir, "shares"),
	}
	res, err := s.SplitToCarriers(sopt)
	if err != nil {
		t.Fatalf("SplitToCarriers: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("应生成 3 个分片, 实际 %d", len(res))
	}

	// 用其中 2 片恢复（缺第 1 片）
	recoverOpt := sopt
	recoverOpt.CarrierPaths = []string{
		filepath.Join(sopt.ShareDir, "share_02.png"),
		filepath.Join(sopt.ShareDir, "share_03.png"),
	}
	recoverOpt.OutputPath = filepath.Join(dir, "recovered")
	rres, err := s.RecoverFromCarriers(recoverOpt)
	if err != nil {
		t.Fatalf("RecoverFromCarriers: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(recoverOpt.OutputPath, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Shamir 分权测试秘密数据" {
		t.Fatalf("恢复内容不一致: %q", got)
	}
	_ = rres
}

func TestServiceShamirInsufficient(t *testing.T) {
	dir := t.TempDir()
	var carriers []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, "c.png")
		writeTestPNG(t, p, 64, 64)
		carriers = append(carriers, p)
	}
	secretPath := filepath.Join(dir, "secret.txt")
	writeSecret(t, secretPath, "data")
	s := newTestService()
	sopt := ShamirOptions{
		Options:      Options{SecretPath: secretPath, Password: []byte("p")},
		Total:        3,
		Threshold:    2,
		CarrierPaths: carriers,
		ShareDir:     filepath.Join(dir, "shares"),
	}
	if _, err := s.SplitToCarriers(sopt); err != nil {
		t.Fatal(err)
	}
	// 只有 1 片 → 恢复失败
	recoverOpt := sopt
	recoverOpt.CarrierPaths = []string{filepath.Join(sopt.ShareDir, "share_01.png")}
	if _, err := s.RecoverFromCarriers(recoverOpt); err == nil {
		t.Fatal("少于阈值应恢复失败")
	}
}

// ---------------------------------------------------------------------------
// 水印
// ---------------------------------------------------------------------------

func TestServiceWatermark(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	out := filepath.Join(dir, "wm.png")
	writeTestPNG(t, src, 64, 64)

	s := newTestService()
	if _, err := s.EmbedWatermark(src, out, "© 2026 StegGo Team"); err != nil {
		t.Fatalf("EmbedWatermark: %v", err)
	}
	mark, err := s.ExtractWatermark(out)
	if err != nil {
		t.Fatalf("ExtractWatermark: %v", err)
	}
	if mark != "© 2026 StegGo Team" {
		t.Fatalf("水印不一致: %q", mark)
	}
	// 无水印图像应报错
	if _, err := s.ExtractWatermark(src); err == nil {
		t.Fatal("无水印应返回错误")
	}
}

// ---------------------------------------------------------------------------
// V1.0 兼容
// ---------------------------------------------------------------------------

func TestServiceV1Compat(t *testing.T) {
	dir := t.TempDir()
	carrierPath := filepath.Join(dir, "v1.png")
	outCarrier := filepath.Join(dir, "v1_out.png")
	secretPath := filepath.Join(dir, "v1secret.txt")
	writeTestPNG(t, carrierPath, 128, 128)
	writeSecret(t, secretPath, "V1.0 载体兼容提取测试")

	// 用 V1.0 流程嵌入
	_, err := v1steg.EmbedImage(carrierPath, outCarrier, secretPath, v1steg.Options{
		BitDepth: 2,
		Password: []byte("v1-pass"),
	})
	if err != nil {
		t.Fatalf("V1 EmbedImage: %v", err)
	}

	// V2.0 service 提取
	s := newTestService()
	extractDir := filepath.Join(dir, "x")
	res, err := s.Extract(baseOpt(outCarrier, "", extractDir, []byte("v1-pass")))
	if err != nil {
		t.Fatalf("V2 提取 V1 载体: %v", err)
	}
	if !res.V1Compat {
		t.Fatal("V1 载体应走 V1 兼容路径")
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "v1secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "V1.0 载体兼容提取测试" {
		t.Fatalf("V1 内容不一致: %q", got)
	}
}
