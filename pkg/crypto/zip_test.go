package crypto

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipBytesRoundTrip(t *testing.T) {
	data := bytes.Repeat([]byte("zip round trip payload "), 100)
	name := "secret.txt"

	z, err := ZipBytes(data, name)
	if err != nil {
		t.Fatalf("ZipBytes: %v", err)
	}
	if !IsZip(z) {
		t.Fatal("输出应为合法 zip 头")
	}
	gotName, gotData, err := UnzipSingleFile(z)
	if err != nil {
		t.Fatalf("UnzipSingleFile: %v", err)
	}
	if gotName != name {
		t.Fatalf("文件名不匹配: %s != %s", gotName, name)
	}
	if !bytes.Equal(gotData, data) {
		t.Fatal("解压内容与原始数据不一致")
	}
}

func TestMaybeZip(t *testing.T) {
	// 超过阈值且压缩有效 → 压缩
	big := bytes.Repeat([]byte("compressible compressible "), 200)
	out, zipped, err := MaybeZip(big, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !zipped {
		t.Fatal("大而可压缩的数据应当启用 ZIP")
	}
	if !IsZip(out) {
		t.Fatal("压缩结果应为 zip")
	}

	// 小数据 → 原样返回
	small := []byte("hi")
	out, zipped, err = MaybeZip(small, 100)
	if err != nil {
		t.Fatal(err)
	}
	if zipped {
		t.Fatal("小数据不应压缩")
	}
	if !bytes.Equal(out, small) {
		t.Fatal("未压缩数据应原样返回")
	}
}

func TestZipDirUnzipRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":        "content-a",
		"sub/b.txt":    "content-b",
		"sub/deep.txt": "content-c",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	z, err := ZipDir(dir)
	if err != nil {
		t.Fatalf("ZipDir: %v", err)
	}
	dest := t.TempDir()
	if err := UnzipBytes(z, dest); err != nil {
		t.Fatalf("UnzipBytes: %v", err)
	}
	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("读取 %s: %v", name, err)
		}
		if string(got) != content {
			t.Fatalf("%s 内容不一致: %q != %q", name, got, content)
		}
	}
}

func TestZipSlipProtection(t *testing.T) {
	// 构造包含 ../ 路径的恶意 zip，UnzipBytes 必须拒绝
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	evil, err := zw.Create("../../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	evil.Write([]byte("pwned"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := UnzipBytes(buf.Bytes(), dest); err == nil {
		t.Fatal("Zip Slip 攻击应当被拒绝")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Fatal("恶意文件不应被写入目标目录之外")
	}
}

func TestUnzipEmpty(t *testing.T) {
	if _, _, err := UnzipSingleFile([]byte("not-a-zip")); err == nil {
		t.Fatal("非法 zip 应报错")
	}
}

func TestIsZipEdge(t *testing.T) {
	if IsZip([]byte("PK\x03\x04junk")) != true {
		t.Fatal("标准 zip 头应识别")
	}
	if IsZip([]byte("PK")) {
		t.Fatal("过短数据不应识别为 zip")
	}
	if IsZip(nil) {
		t.Fatal("空数据不应识别为 zip")
	}
}

func TestZipBytesToMemory(t *testing.T) {
	data := []byte("memory round trip")
	z, err := ZipBytes(data, "f.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnzipBytesToMemory(z)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("内存解压内容不一致")
	}
}

func TestZipNameUnicode(t *testing.T) {
	data := []byte("中文文件名测试")
	z, err := ZipBytes(data, "秘密文件-文档.txt")
	if err != nil {
		t.Fatal(err)
	}
	name, got, err := UnzipSingleFile(z)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(name, "秘密文件") {
		t.Fatalf("中文文件名异常: %s", name)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("内容不一致")
	}
}
