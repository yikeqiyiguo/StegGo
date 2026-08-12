package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := []byte("StegGo 银行级加密链路测试数据 0123456789")
	pw := []byte("correct horse battery staple")

	ct, err := Encrypt(plain, pw)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// 布局：salt(16) + nonce(12) + ciphertext
	if len(ct) < SaltSize+NonceSize+TagSize {
		t.Fatalf("密文过短: %d", len(ct))
	}
	pt, err := Decrypt(ct, pw)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatal("解密结果与明文不一致")
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	ct, err := Encrypt([]byte("secret"), []byte("pw-1"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(ct, []byte("pw-2")); err == nil {
		t.Fatal("错误密码应当解密失败")
	}
}

func TestDecryptTampered(t *testing.T) {
	ct, err := Encrypt([]byte("secret"), []byte("pw"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// 篡改最后一个字节
	ct[len(ct)-1] ^= 0xFF
	if _, err := Decrypt(ct, []byte("pw")); err == nil {
		t.Fatal("篡改数据应当解密失败（GCM 认证应拦截）")
	}
}

func TestDecryptTooShort(t *testing.T) {
	if _, err := Decrypt([]byte("abc"), []byte("pw")); err == nil {
		t.Fatal("过短密文应当报错")
	}
}

func TestEncryptUnique(t *testing.T) {
	// 相同明文两次加密结果必须不同（随机盐/随机数）
	c1, err := Encrypt([]byte("same"), []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Encrypt([]byte("same"), []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(c1, c2) {
		t.Fatal("相同明文两次加密结果不应相同")
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	k1 := DeriveKey([]byte("pw"), []byte("salt"), 1000)
	k2 := DeriveKey([]byte("pw"), []byte("salt"), 1000)
	if !bytes.Equal(k1, k2) {
		t.Fatal("相同参数派生密钥应当一致")
	}
	if len(k1) != KeySize {
		t.Fatalf("密钥长度应为 %d, 实际 %d", KeySize, len(k1))
	}
	k3 := DeriveKey([]byte("pw"), []byte("salt2"), 1000)
	if bytes.Equal(k1, k3) {
		t.Fatal("不同盐派生密钥不应相同")
	}
}

func TestWipe(t *testing.T) {
	b := []byte("sensitive password data")
	Wipe(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("Wipe 后第 %d 字节未被清零: %d", i, v)
		}
	}
	Wipe(nil) // 不应 panic
}

func TestEncryptChunkDecryptChunk(t *testing.T) {
	key := DeriveKey([]byte("pw"), []byte("chunk-salt"), 1000)
	plain := bytes.Repeat([]byte("ABCDEF0123456789"), 4096) // 64KB

	ct, err := EncryptChunk(plain, key)
	if err != nil {
		t.Fatalf("EncryptChunk: %v", err)
	}
	if len(ct) <= len(plain) {
		t.Fatal("分片加密后长度应大于明文（含 nonce+tag）")
	}
	pt, err := DecryptChunk(ct, key)
	if err != nil {
		t.Fatalf("DecryptChunk: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatal("分片解密结果与明文不一致")
	}
}

func TestEncryptChunkWrongKey(t *testing.T) {
	k1 := DeriveKey([]byte("pw-a"), []byte("salt"), 1000)
	k2 := DeriveKey([]byte("pw-b"), []byte("salt"), 1000)
	ct, err := EncryptChunk([]byte("data"), k1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptChunk(ct, k2); err == nil {
		t.Fatal("错误密钥应当解密失败")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual([]byte("abc"), []byte("abc")) {
		t.Fatal("相等数据应返回 true")
	}
	if ConstantTimeEqual([]byte("abc"), []byte("abd")) {
		t.Fatal("不等数据应返回 false")
	}
}

func TestDefaultIterationsHigh(t *testing.T) {
	if DefaultIterations < 200_000 {
		t.Fatalf("默认迭代次数过低，不满足银行级要求: %d", DefaultIterations)
	}
}
