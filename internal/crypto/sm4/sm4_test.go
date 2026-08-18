package sm4

import (
	"bytes"
	"crypto/cipher"
	"encoding/hex"
	"testing"
)

// TestSM4StandardVector 标准测试向量（GB/T 32907-2016 附录）：
// 密钥 0123456789abcdeffedcba9876543210
// 明文 0123456789abcdeffedcba9876543210
// 密文 681edf34d206965e86b3e94f536e4246
func TestSM4StandardVector(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	pt, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	want, _ := hex.DecodeString("681edf34d206965e86b3e94f536e4246")

	block, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ct := make([]byte, BlockSize)
	block.Encrypt(ct, pt)
	if !bytes.Equal(ct, want) {
		t.Fatalf("加密结果不匹配:\n got %x\nwant %x", ct, want)
	}

	back := make([]byte, BlockSize)
	block.Decrypt(back, ct)
	if !bytes.Equal(back, pt) {
		t.Fatalf("解密结果不匹配:\n got %x\nwant %x", back, pt)
	}
}

// TestSM4RoundTrip 任意密钥往返测试
func TestSM4RoundTrip(t *testing.T) {
	for _, k := range [][]byte{
		bytes.Repeat([]byte{0x00}, 16),
		bytes.Repeat([]byte{0xff}, 16),
		[]byte("StegGo-SM4-Key01"),
	} {
		block, err := NewCipher(k)
		if err != nil {
			t.Fatalf("NewCipher: %v", err)
		}
		pt := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
		ct := make([]byte, BlockSize)
		back := make([]byte, BlockSize)
		block.Encrypt(ct, pt)
		block.Decrypt(back, ct)
		if !bytes.Equal(back, pt) {
			t.Fatalf("往返失败 key=%x", k)
		}
	}
}

// TestSM4KeyError 错误密钥长度
func TestSM4KeyError(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("期望密钥长度错误，实际返回 nil")
	}
	if _, err := NewCipher(make([]byte, 32)); err == nil {
		t.Fatal("期望 32 字节密钥报错，实际返回 nil")
	}
}

// TestSM4GCM SM4 配合 GCM 模式（与 AES-GCM 布局一致：[nonce][ciphertext+tag]）
func TestSM4GCM(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	block, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}
	nonce := bytes.Repeat([]byte{0x11}, gcm.NonceSize())
	pt := []byte("SM4-GCM 国密模式兼容测试 payload 数据")
	ct := gcm.Seal(nil, nonce, pt, nil)
	back, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatalf("GCM Open: %v", err)
	}
	if !bytes.Equal(back, pt) {
		t.Fatal("SM4-GCM 往返数据不一致")
	}
	if gcm.NonceSize() != 12 || gcm.Overhead() != 16 {
		t.Fatalf("GCM 参数不兼容: nonce=%d tag=%d", gcm.NonceSize(), gcm.Overhead())
	}
}
