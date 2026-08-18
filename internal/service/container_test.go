package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestServiceContainer 通过 service 层文件级往返。
func TestServiceContainer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cover.png")
	dst := filepath.Join(dir, "cover.png.sg")
	back := filepath.Join(dir, "restored.png")

	orig := []byte("fake-png-bytes-for-container-test")
	if err := os.WriteFile(src, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := New().ContainerEncrypt(src, dst, []byte("pw"), true)
	if err != nil {
		t.Fatalf("ContainerEncrypt: %v", err)
	}
	if !res.UseSM4 || res.Name != "cover.png" {
		t.Fatalf("结果错误: %+v", res)
	}
	res2, err := New().ContainerDecrypt(dst, back, []byte("pw"))
	if err != nil {
		t.Fatalf("ContainerDecrypt: %v", err)
	}
	if res2.Name != "cover.png" {
		t.Fatalf("文件名错误: %s", res2.Name)
	}
	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatal("还原内容不一致")
	}
}

// TestServiceContainerWrongPassword service 层错误密码明确报错。
func TestServiceContainerWrongPassword(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.bin")
	dst := filepath.Join(dir, "a.bin.sg")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New().ContainerEncrypt(src, dst, []byte("pw"), false); err != nil {
		t.Fatalf("ContainerEncrypt: %v", err)
	}
	if _, err := New().ContainerDecrypt(dst, filepath.Join(dir, "x.bin"), []byte("wrong")); err == nil {
		t.Fatal("错误密码应解密失败")
	}
}
