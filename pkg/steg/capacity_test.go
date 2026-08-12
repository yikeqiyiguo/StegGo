package steg

import (
	"os"
	"testing"

	"steggo/pkg/carrier"
)

func TestCheckImageCapacity(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cap.png"
	img := newTestImage(100, 80, 13)
	if err := carrier.SaveImage(img, path); err != nil {
		t.Fatal(err)
	}

	r, err := CheckImageCapacity(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Width != 100 || r.Height != 80 {
		t.Fatalf("尺寸错误: %+v", r)
	}
	wantBits := int64(100 * 80 * 3 * 2)
	if r.MaxBits != wantBits {
		t.Fatalf("MaxBits 错误: %d != %d", r.MaxBits, wantBits)
	}
	if r.MaxBytes != wantBits/8 {
		t.Fatal("MaxBytes 计算错误")
	}
	if r.Usable != r.MaxBytes-r.Overhead {
		t.Fatal("Usable 计算错误")
	}
}

func TestCheckImageCapacityInvalidDepth(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cap2.png"
	if err := carrier.SaveImage(newTestImage(32, 32, 2), path); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckImageCapacity(path, 0); err == nil {
		t.Fatal("位深度 0 应报错")
	}
	if _, err := CheckImageCapacity(path, 5); err == nil {
		t.Fatal("位深度 5 应报错")
	}
}

func TestCapacityMatrix(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/matrix.png"
	if err := carrier.SaveImage(newTestImage(64, 64, 4), path); err != nil {
		t.Fatal(err)
	}
	matrix, err := CapacityMatrix(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 4 {
		t.Fatalf("矩阵应有 4 档位深度, 实际 %d", len(matrix))
	}
	// 位深度越大容量越大
	for i := 1; i < len(matrix); i++ {
		if matrix[i].MaxBytes <= matrix[i-1].MaxBytes {
			t.Fatal("容量应随位深度递增")
		}
	}
}

func TestCheckGenericCapacity(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/any.bin"
	content := make([]byte, 1234)
	for i := range content {
		content[i] = byte(i)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	sz, err := CheckGenericCapacity(path)
	if err != nil {
		t.Fatal(err)
	}
	if sz != 1234 {
		t.Fatalf("容量应为 1234, 实际 %d", sz)
	}
}
