package steg

import (
	"bytes"
	"testing"

	"steggo/pkg/crypto"
)

func TestHeaderRoundTrip(t *testing.T) {
	h := &Header{
		Version:   2,
		Flags:     flagZIP,
		BitDepth:  3,
		Name:      "秘密文件-数据.bin",
		Salt:      bytes.Repeat([]byte{0x11}, crypto.SaltSize),
		Nonce:     bytes.Repeat([]byte{0x22}, crypto.NonceSize),
		CipherLen: 1234,
	}
	copy(h.CipherSum[:], bytes.Repeat([]byte{0x33}, 32))

	stream := EncodeHeader(h)
	parsed, headLen, err := ParseHeader(stream)
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if headLen != len(stream) {
		t.Fatalf("头部长度计算错误: %d != %d", headLen, len(stream))
	}
	if parsed.Version != 2 {
		t.Fatalf("Version 错误: %d", parsed.Version)
	}
	if parsed.BitDepth != 3 {
		t.Fatalf("BitDepth 错误: %d", parsed.BitDepth)
	}
	if parsed.Name != h.Name {
		t.Fatalf("Name 错误: %q != %q", parsed.Name, h.Name)
	}
	if parsed.CipherLen != 1234 {
		t.Fatalf("CipherLen 错误: %d", parsed.CipherLen)
	}
	if !bytes.Equal(parsed.Salt, h.Salt) || !bytes.Equal(parsed.Nonce, h.Nonce) {
		t.Fatal("Salt/Nonce 不一致")
	}
	if parsed.CipherSum != h.CipherSum {
		t.Fatal("CipherSum 不一致")
	}
	if err := ValidateHeader(parsed); err != nil {
		t.Fatalf("ValidateHeader: %v", err)
	}
}

func TestParseHeaderBadMagic(t *testing.T) {
	_, _, err := ParseHeader([]byte("WRONGMAGIC-garbage"))
	if err == nil {
		t.Fatal("错误 magic 应报错")
	}
}

func TestParseHeaderTooShort(t *testing.T) {
	if _, _, err := ParseHeader([]byte("STEGGO2")); err == nil {
		t.Fatal("过短数据应报错")
	}
}

func TestParseHeaderInvalidBitDepth(t *testing.T) {
	stream := EncodeHeader(&Header{
		Version:   2,
		BitDepth:  9,
		Name:      "x",
		Salt:      make([]byte, crypto.SaltSize),
		Nonce:     make([]byte, crypto.NonceSize),
		CipherLen: 10,
	})
	_, _, err := ParseHeader(stream)
	if err == nil {
		t.Fatal("非法位深度应报错")
	}
}

func TestParseHeaderZeroCipherLen(t *testing.T) {
	h := &Header{
		Version:   2,
		BitDepth:  2,
		Name:      "x",
		Salt:      make([]byte, crypto.SaltSize),
		Nonce:     make([]byte, crypto.NonceSize),
		CipherLen: 0,
	}
	if _, _, err := ParseHeader(EncodeHeader(h)); err == nil {
		t.Fatal("零密文长度应报错")
	}
}

func TestBuildPayload(t *testing.T) {
	h := &Header{
		Version:   2,
		BitDepth:  2,
		Name:      "payload.bin",
		Salt:      make([]byte, crypto.SaltSize),
		Nonce:     make([]byte, crypto.NonceSize),
		CipherLen: 5,
	}
	cipher := []byte("hello")
	payload := BuildPayload(h, cipher)
	parsed, off, err := ParseHeader(payload)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.CipherLen != 5 {
		t.Fatalf("CipherLen 错误: %d", parsed.CipherLen)
	}
	if !bytes.Equal(payload[off:], cipher) {
		t.Fatal("密文段不一致")
	}
}

func TestMagicV2(t *testing.T) {
	if len(MagicV2) != 8 {
		t.Fatalf("magic 长度应为 8: %q", MagicV2)
	}
	if string(MagicV2) != "STEGGO2A" {
		t.Fatalf("magic 值错误: %q", MagicV2)
	}
}

func TestWipeHeader(t *testing.T) {
	h := &Header{
		Salt:  make([]byte, crypto.SaltSize),
		Nonce: make([]byte, crypto.NonceSize),
		Name:  "sensitive",
	}
	for i := range h.Salt {
		h.Salt[i] = 0xAB
	}
	WipeHeader(h)
	for _, v := range h.Salt {
		if v != 0 {
			t.Fatal("WipeHeader 未清零 Salt")
		}
	}
	if h.Name != "" {
		t.Fatal("WipeHeader 未清理 Name")
	}
}
