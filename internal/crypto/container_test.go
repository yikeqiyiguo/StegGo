package crypto

import (
	"bytes"
	"testing"
)

// TestContainerRoundTrip .sg 容器往返（AES + SM4）。
func TestContainerRoundTrip(t *testing.T) {
	carrier := bytes.Repeat([]byte("PNG-cover-data-"), 64)
	for _, useSM4 := range []bool{false, true} {
		container, meta, err := EncryptContainer(carrier, "cover.png", []byte("pass-123"), useSM4)
		if err != nil {
			t.Fatalf("useSM4=%v EncryptContainer: %v", useSM4, err)
		}
		if !IsContainer(container) {
			t.Fatalf("useSM4=%v 魔数识别失败", useSM4)
		}
		if meta.UseSM4 != useSM4 {
			t.Fatalf("useSM4=%v meta 标记不一致", useSM4)
		}
		if meta.Name != "cover.png" || meta.Size != int64(len(carrier)) {
			t.Fatalf("useSM4=%v meta 错误: %+v", useSM4, meta)
		}
		plain, meta2, err := DecryptContainer(container, []byte("pass-123"))
		if err != nil {
			t.Fatalf("useSM4=%v DecryptContainer: %v", useSM4, err)
		}
		if !bytes.Equal(plain, carrier) {
			t.Fatalf("useSM4=%v 解密内容不一致", useSM4)
		}
		if meta2.Name != "cover.png" {
			t.Fatalf("useSM4=%v 文件名未还原", useSM4)
		}
	}
}

// TestContainerWrongPassword 错误密码 / 篡改检测。
func TestContainerWrongPassword(t *testing.T) {
	container, _, err := EncryptContainer([]byte("secret-carrier-bytes"), "a.bin", []byte("right"), false)
	if err != nil {
		t.Fatalf("EncryptContainer: %v", err)
	}
	if _, _, err := DecryptContainer(container, []byte("wrong")); err == nil {
		t.Fatal("错误密码应解密失败")
	}
	// 篡改一个字节
	tampered := append([]byte(nil), container...)
	tampered[len(tampered)-1] ^= 0x01
	if _, _, err := DecryptContainer(tampered, []byte("right")); err == nil {
		t.Fatal("篡改数据应解密失败（GCM 认证）")
	}
	// 非容器
	if _, _, err := DecryptContainer([]byte("not a container"), []byte("right")); err == nil {
		t.Fatal("非容器数据应报错")
	}
}
