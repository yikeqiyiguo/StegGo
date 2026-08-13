package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"os"
)

// 配置/日志/小数据加密统一使用 AES-256-GCM，密钥固定 32 字节。
const (
	// ToolKeySize AES-256 密钥长度
	ToolKeySize = 32
	// ToolNonceSize GCM 随机数长度
	ToolNonceSize = 12
)

// EncryptBytes 使用 32 字节密钥加密数据，返回 [nonce][ciphertext+tag]。
func EncryptBytes(key, data []byte) ([]byte, error) {
	if len(key) != ToolKeySize {
		return nil, errors.New("加密密钥必须为 32 字节")
	}
	block, err := aes.NewCipher(key)
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
	return gcm.Seal(nonce, nonce, data, nil), nil
}

// DecryptBytes 解密 EncryptBytes 的输出。
func DecryptBytes(key, data []byte) ([]byte, error) {
	if len(key) != ToolKeySize {
		return nil, errors.New("加密密钥必须为 32 字节")
	}
	if len(data) < ToolNonceSize+16 {
		return nil, errors.New("密文数据过短")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// LoadOrCreateKey 从文件加载 32 字节密钥；文件不存在时随机生成并落盘。
func LoadOrCreateKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) == ToolKeySize {
		return b, nil
	}
	key := make([]byte, ToolKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
