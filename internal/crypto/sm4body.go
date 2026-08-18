package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"steggo/internal/crypto/sm4"
	v1crypto "steggo/pkg/crypto"
)

// flagSM4 V3 头部 flags 第 7 位：载荷使用 SM4-GCM 国密算法加密。
const flagSM4 byte = 1 << 7

// sm4KeySize SM4 密钥长度（16 字节）
const sm4KeySize = 16

// newCipherBlock 按算法名称创建分组密码。
// SM4 块大小 16 字节，与 AES 相同，可共用 GCM 模式（nonce 12B + tag 16B）。
func newCipherBlock(algo string, key []byte) (cipher.Block, error) {
	switch algo {
	case "sm4":
		return sm4.NewCipher(key)
	default:
		return aes.NewCipher(key)
	}
}

// sealBody 用给定密钥与 salt 加密，输出布局与 V1/V2 完全一致：
//
//	[salt 16B][nonce 12B][ciphertext+tag]
//
// AES-256-GCM（默认）与 SM4-GCM（useSM4=true）共用同一布局，
// 因此 V3 头部 [salt][nonce] 提取逻辑无需任何改动。
func sealBody(key []byte, useSM4 bool, salt, data []byte) ([]byte, error) {
	algo := "aes"
	keyLen := v1crypto.KeySize
	if useSM4 {
		algo = "sm4"
		keyLen = sm4KeySize
	}
	if len(key) < keyLen {
		return nil, errors.New("加密密钥长度不足")
	}
	block, err := newCipherBlock(algo, key[:keyLen])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	out := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// encryptBodyWithKey 用给定密钥直接加密（Kyber 混合加密使用）：
//
//	[salt 16B][nonce 12B][ciphertext+tag]
//
// salt 为随机字节，仅维持与 V3 头部布局一致（密钥不再由 salt 派生）。
func encryptBodyWithKey(key []byte, useSM4 bool, data []byte) ([]byte, error) {
	salt := make([]byte, v1crypto.SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return sealBody(key, useSM4, salt, data)
}

// encryptBody 加密载荷体（密码经 PBKDF2 派生密钥）：
//
//	[salt 16B][nonce 12B][ciphertext+tag]
//
// 注意：派生密钥与输出的 salt 必须为同一字节（解密时从密文头部取 salt 派生）。
func encryptBody(secret []byte, useSM4 bool, data []byte) ([]byte, error) {
	salt := make([]byte, v1crypto.SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	keyLen := v1crypto.KeySize
	if useSM4 {
		keyLen = sm4KeySize
	}
	// DeriveKey 固定返回 32 字节，SM4 仅取前 16 字节。
	key := v1crypto.DeriveKey(secret, salt, v1crypto.DefaultIterations)[:keyLen]
	defer v1crypto.Wipe(key)
	return sealBody(key, useSM4, salt, data)
}

// decryptBodyWithKey 用给定密钥直接解密（Kyber 混合加密使用）。
func decryptBodyWithKey(key []byte, useSM4 bool, data []byte) ([]byte, error) {
	if len(data) < v1crypto.SaltSize+v1crypto.NonceSize+v1crypto.TagSize {
		return nil, errors.New("密文数据过短，载体可能已损坏")
	}
	algo := "aes"
	keyLen := v1crypto.KeySize
	if useSM4 {
		algo = "sm4"
		keyLen = sm4KeySize
	}
	if len(key) < keyLen {
		return nil, errors.New("解密密钥长度不足")
	}
	nonce := data[v1crypto.SaltSize : v1crypto.SaltSize+v1crypto.NonceSize]
	ciphertext := data[v1crypto.SaltSize+v1crypto.NonceSize:]
	block, err := newCipherBlock(algo, key[:keyLen])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// 不区分密码错误与数据损坏，避免泄露信息
		return nil, errors.New("解密失败：密码错误或数据已被篡改")
	}
	return plaintext, nil
}

// decryptBody 解密 encryptBody 的输出（布局兼容 AES-GCM 与 SM4-GCM）。
func decryptBody(secret []byte, useSM4 bool, data []byte) ([]byte, error) {
	if len(data) < v1crypto.SaltSize+v1crypto.NonceSize+v1crypto.TagSize {
		return nil, errors.New("密文数据过短，载体可能已损坏")
	}
	keyLen := v1crypto.KeySize
	if useSM4 {
		keyLen = sm4KeySize
	}
	salt := data[:v1crypto.SaltSize]
	key := v1crypto.DeriveKey(secret, salt, v1crypto.DefaultIterations)[:keyLen]
	defer v1crypto.Wipe(key)
	return decryptBodyWithKey(key, useSM4, data)
}
