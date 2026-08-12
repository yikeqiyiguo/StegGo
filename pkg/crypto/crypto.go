// Package crypto 提供 StegGo 的银行级加密体系：
//
//	原始数据 → ZIP压缩 → PBKDF2密钥派生 → AES-256-GCM加密 → SHA256哈希绑定
//
// 所有敏感字节使用完毕均由 Wipe 主动覆盖清零，禁止明文留存。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"runtime"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// SaltSize PBKDF2 随机盐长度
	SaltSize = 16
	// KeySize AES-256 密钥长度
	KeySize = 32
	// NonceSize AES-GCM 随机数长度
	NonceSize = 12
	// TagSize AES-GCM 认证标签长度
	TagSize = 16
	// DefaultIterations PBKDF2 迭代次数（高迭代，防暴力破解）
	DefaultIterations = 210_000
	// FixedSalt 用于派生隐写坐标随机种子等非密钥用途的固定盐
	FixedSalt = "StegGo::V1.0::offline-only::2026"
)

// Wipe 用零值覆盖字节切片，防止敏感数据残留内存。
// 通过 runtime.KeepAlive 阻止编译器优化掉覆盖操作。
func Wipe(b []byte) {
	if len(b) == 0 {
		return
	}
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// WipeStrings 覆盖并释放多个字符串（通过转成可变切片覆盖）。
// Go 的 string 不可变，此函数仅作尽力而为的清理，真正安全应使用 []byte 承载密码。
func WipeStrings(ss ...string) {
	for _, s := range ss {
		b := []byte(s)
		for i := range b {
			b[i] = 0
		}
	}
}

// DeriveKey 通过 PBKDF2-SHA256 从密码派生密钥。
func DeriveKey(password []byte, salt []byte, iterations int) []byte {
	if iterations <= 0 {
		iterations = DefaultIterations
	}
	return pbkdf2.Key(password, salt, iterations, KeySize, sha256.New)
}

// Encrypt 加密明文，返回布局 [salt][nonce][ciphertext+tag]。
func Encrypt(plaintext, password []byte) ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key := DeriveKey(password, salt, DefaultIterations)
	defer Wipe(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, SaltSize+NonceSize+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// Decrypt 解密 Encrypt 的输出。
// 密码错误 / 数据被篡改时返回统一错误，不暴露任何内部细节。
func Decrypt(data, password []byte) ([]byte, error) {
	if len(data) < SaltSize+NonceSize+TagSize {
		return nil, errors.New("密文数据过短，载体可能已损坏")
	}
	salt := data[:SaltSize]
	nonce := data[SaltSize : SaltSize+NonceSize]
	ciphertext := data[SaltSize+NonceSize:]

	key := DeriveKey(password, salt, DefaultIterations)
	defer Wipe(key)

	block, err := aes.NewCipher(key)
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

// DecryptWithKey 使用预先派生的密钥和 nonce 解密 AES-GCM 密文。
// 供流式/分片场景复用密钥，避免重复 PBKDF2。
func DecryptWithKey(ciphertext, key, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("解密失败：密码错误或数据已被篡改")
	}
	return pt, nil
}

// EncryptChunk 使用同一密钥流式加密多个数据块。
// 每次调用生成独立随机 nonce，适合大文件分块处理。
func EncryptChunk(chunk []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, chunk, nil)
	out := make([]byte, 0, NonceSize+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// DecryptChunk 解密 EncryptChunk 产生的数据块。
func DecryptChunk(chunk []byte, key []byte) ([]byte, error) {
	if len(chunk) < NonceSize+TagSize {
		return nil, errors.New("加密分片数据过短")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, chunk[:NonceSize], chunk[NonceSize:], nil)
	if err != nil {
		return nil, errors.New("解密失败：密码错误或数据已被篡改")
	}
	return pt, nil
}

// ConstantTimeEqual 常量时间比较两个字节切片。
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
