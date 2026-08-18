package crypto

import (
	"bytes"
	"testing"
)

// TestKyberAvailable 验证后量子实现已注册（标准库 ML-KEM-768）。
func TestKyberAvailable(t *testing.T) {
	if !KyberAvailable() {
		t.Fatal("ML-KEM 后量子实现未注册")
	}
	kem, err := NewKyberKEM()
	if err != nil {
		t.Fatal(err)
	}
	if kem.Name() != "ML-KEM-768" {
		t.Fatalf("算法名错误: %s", kem.Name())
	}
}

// TestKyberKEMRoundtrip 验证封装/解封装共享密钥一致。
func TestKyberKEMRoundtrip(t *testing.T) {
	kem, err := NewKyberKEM()
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := kem.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != KyberPubKeySize {
		t.Fatalf("公钥长度 %d != %d", len(pub), KyberPubKeySize)
	}
	if len(priv) != KyberPrivKeySize {
		t.Fatalf("私钥长度 %d != %d", len(priv), KyberPrivKeySize)
	}
	ct, shared, err := kem.Encapsulate(pub)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) != KyberCipherSize {
		t.Fatalf("封装密文长度 %d != %d", len(ct), KyberCipherSize)
	}
	if len(shared) != KyberSharedSize {
		t.Fatalf("共享密钥长度 %d != %d", len(shared), KyberSharedSize)
	}
	got, err := kem.Decapsulate(priv, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, shared) {
		t.Fatal("封装/解封装共享密钥不一致")
	}
}

// TestKyberWrapRoundtrip 验证 WrapKey/UnwrapKey 恢复 AES 主密钥。
func TestKyberWrapRoundtrip(t *testing.T) {
	wrap, err := GenerateKyberKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	wrapped, err := wrap.WrapKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrapped) != KyberCipherSize+32 {
		t.Fatalf("封装输出长度 %d", len(wrapped))
	}
	restored, err := (&KyberWrap{PrivKey: wrap.PrivKey}).UnwrapKey(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, key) {
		t.Fatal("WrapKey/UnwrapKey 未恢复原密钥")
	}
}

// TestKyberPayloadRoundtrip 验证 Kyber 混合加密在完整载荷链路的往返。
func TestKyberPayloadRoundtrip(t *testing.T) {
	pub, priv, err := (&mlkemKEM{}).GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("post-quantum-secret")
	payload, meta, err := BuildPayload(secret, &BuildOptions{
		Password:  []byte("pwd"),
		Name:      "secret.bin",
		Algorithm: "lsb",
		BitDepth:  1,
		KyberPub:  pub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || !meta.Kyber {
		t.Fatal("meta.Kyber 应为 true")
	}

	plain, pmeta, err := ParsePayload(payload, &ParseOptions{
		Password:  []byte("pwd"),
		KyberPriv: priv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, secret) {
		t.Fatal("Kyber 载荷往返解密内容不一致")
	}
	if pmeta == nil || !pmeta.Kyber {
		t.Fatal("解析 meta.Kyber 应为 true")
	}
}

// TestKyberPayloadWrongPriv 验证私钥不匹配时解封装失败（不泄露明文）。
func TestKyberPayloadWrongPriv(t *testing.T) {
	pub, _, err := (&mlkemKEM{}).GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPriv, err := (&mlkemKEM{}).GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload, _, err := BuildPayload([]byte("x"), &BuildOptions{
		Password:  []byte("pwd"),
		Name:      "s",
		Algorithm: "lsb",
		BitDepth:  1,
		KyberPub:  pub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParsePayload(payload, &ParseOptions{
		Password:  []byte("pwd"),
		KyberPriv: wrongPriv,
	}); err == nil {
		t.Fatal("私钥不匹配应解封装失败")
	}
}

// TestKyberPayloadNoPriv 验证缺少私钥时报明确错误。
func TestKyberPayloadNoPriv(t *testing.T) {
	pub, _, err := (&mlkemKEM{}).GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload, _, err := BuildPayload([]byte("x"), &BuildOptions{
		Password:  []byte("pwd"),
		Name:      "s",
		Algorithm: "lsb",
		BitDepth:  1,
		KyberPub:  pub,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ParsePayload(payload, &ParseOptions{Password: []byte("pwd")})
	if err == nil {
		t.Fatal("缺少私钥应报错")
	}
}

// TestKyberUnsupportedSize 验证非法密钥长度的 WrapKey 报错。
func TestKyberUnsupportedSize(t *testing.T) {
	w := &KyberWrap{PubKey: make([]byte, KyberPubKeySize)}
	if _, err := w.WrapKey(make([]byte, 16)); err == nil {
		t.Fatal("非 32 字节密钥应报错")
	}
	w2 := &KyberWrap{PrivKey: make([]byte, KyberPrivKeySize)}
	if _, err := w2.UnwrapKey(make([]byte, 4)); err == nil {
		t.Fatal("过短封装数据应报错")
	}
}
