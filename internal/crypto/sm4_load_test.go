package crypto

import (
	"bytes"
	"testing"

	v1crypto "steggo/pkg/crypto"
)

// TestSM4PayloadRoundTrip SM4 载荷完整往返：嵌入 → 解析 → 解密。
func TestSM4PayloadRoundTrip(t *testing.T) {
	data := []byte("SM4 国密载荷测试 - payload data for roundtrip")
	password := []byte("test-pass-123")

	for _, useSM4 := range []bool{false, true} {
		payload, meta, err := BuildPayload(data, &BuildOptions{
			Name:      "secret.txt",
			Algorithm: "lsb",
			BitDepth:  1,
			Compress:  false,
			Password:  password,
			UseSM4:    useSM4,
		})
		if err != nil {
			t.Fatalf("useSM4=%v BuildPayload: %v", useSM4, err)
		}

		// 检查 flags 位
		head, _, err := ParseV3Header(payload)
		if err != nil {
			t.Fatalf("useSM4=%v ParseV3Header: %v", useSM4, err)
		}
		if got := head.Flags&flagSM4 != 0; got != useSM4 {
			t.Fatalf("useSM4=%v flags SM4 位不匹配: flags=0x%02x", useSM4, head.Flags)
		}

		back, meta2, err := ParsePayload(payload, &ParseOptions{Password: password})
		if err != nil {
			t.Fatalf("useSM4=%v ParsePayload: %v", useSM4, err)
		}
		if !bytes.Equal(back, data) {
			t.Fatalf("useSM4=%v 解密数据不一致: %q", useSM4, back)
		}
		if meta.Name != meta2.Name || meta.Algorithm != meta2.Algorithm {
			t.Fatalf("useSM4=%v meta 不一致", useSM4)
		}
	}
}

// TestSM4PayloadWrongPassword SM4 载荷错误密码统一报错。
func TestSM4PayloadWrongPassword(t *testing.T) {
	payload, _, err := BuildPayload([]byte("secret"), &BuildOptions{
		Name: "a.txt", Algorithm: "dct", BitDepth: 2,
		Password: []byte("right-pass"), UseSM4: true,
	})
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}
	if _, _, err := ParsePayload(payload, &ParseOptions{Password: []byte("wrong-pass")}); err == nil {
		t.Fatal("错误密码应解密失败")
	}
}

// TestSM4EncryptBodyCompat encryptBody 布局与 v1crypto.Encrypt 一致（salt16+nonce12+rest）。
func TestSM4EncryptBodyCompat(t *testing.T) {
	secret := []byte("compose-secret")
	for _, useSM4 := range []bool{false, true} {
		out, err := encryptBody(secret, useSM4, []byte("hello sm4 body"))
		if err != nil {
			t.Fatalf("useSM4=%v: %v", useSM4, err)
		}
		if len(out) != v1crypto.SaltSize+v1crypto.NonceSize+len("hello sm4 body")+v1crypto.TagSize {
			t.Fatalf("useSM4=%v 布局长度异常: %d", useSM4, len(out))
		}
		back, err := decryptBody(secret, useSM4, out)
		if err != nil {
			t.Fatalf("useSM4=%v decryptBody: %v", useSM4, err)
		}
		if string(back) != "hello sm4 body" {
			t.Fatalf("useSM4=%v 数据不一致", useSM4)
		}
	}
}
