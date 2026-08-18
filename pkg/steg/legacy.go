package steg

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"image"

	"crypto/sha256"
	"golang.org/x/crypto/pbkdf2"
)

// =============================================================
// V1 旧版载体兼容层
//
// 旧版图片 LSB 格式：
//
//	[payload_len uint64 BE 8B][payload]
//
// 旧版载荷格式（payload.go）：
//
//	[magic "STEGGO01" 8B][name_len u16][name][data_len u32][enc_data]
//
// 旧版加密格式：PBKDF2(100k) + AES-GCM，[salt 16B][nonce 12B][ciphertext]
// =============================================================

var legacyMagic = []byte("STEGGO01")

// tryLegacyImage 按旧版顺序 LSB(1bit) 读取载荷。
func tryLegacyImage(img *image.NRGBA, password []byte) ([]byte, bool) {
	if img == nil {
		return nil, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w*h*3 < 64 {
		return nil, false
	}
	// 顺序读取 8B 长度
	lenBits := make([]byte, 64)
	bi := 0
	for y := b.Min.Y; y < b.Max.Y && bi < 64; y++ {
		for x := b.Min.X; x < b.Max.X && bi < 64; x++ {
			c := img.NRGBAAt(x, y)
			lenBits[bi] = c.R & 1
			bi++
			if bi < 64 {
				lenBits[bi] = c.G & 1
				bi++
			}
			if bi < 64 {
				lenBits[bi] = c.B & 1
				bi++
			}
		}
	}
	payloadLen := int(binary.BigEndian.Uint64(BitsToBytes(lenBits)))
	if payloadLen <= 0 || payloadLen > 100*1024*1024 {
		return nil, false
	}
	// 读取 payload 全部位
	needBits := payloadLen * 8
	if w*h*3 < 64+needBits {
		return nil, false
	}
	rawBits := make([]byte, 0, 64+needBits)
	bi = 0
	for y := b.Min.Y; y < b.Max.Y && bi < 64+needBits; y++ {
		for x := b.Min.X; x < b.Max.X && bi < 64+needBits; x++ {
			c := img.NRGBAAt(x, y)
			rawBits = append(rawBits, c.R&1, c.G&1, c.B&1)
			bi += 3
		}
	}
	payload := BitsToBytes(rawBits[64 : 64+needBits])
	return payload, true
}

// decodeLegacy 解析旧版载荷格式。
func decodeLegacy(payload []byte) (string, []byte, error) {
	if len(payload) < 8+2+4 {
		return "", nil, errors.New("旧版载荷过短")
	}
	for i, m := range legacyMagic {
		if payload[i] != m {
			return "", nil, errors.New("非 StegGo 旧版载荷")
		}
	}
	off := 8
	nameLen := int(binary.BigEndian.Uint16(payload[off : off+2]))
	off += 2
	if len(payload) < off+nameLen+4 {
		return "", nil, errors.New("旧版载荷名称截断")
	}
	name := string(payload[off : off+nameLen])
	off += nameLen
	dataLen := int(binary.BigEndian.Uint32(payload[off : off+4]))
	off += 4
	if len(payload) < off+dataLen {
		return "", nil, errors.New("旧版载荷数据截断")
	}
	data := make([]byte, dataLen)
	copy(data, payload[off:off+dataLen])
	return name, data, nil
}

// legacyDecrypt 兼容旧版加密格式（PBKDF2 100k 次迭代）。
func legacyDecrypt(data, password []byte) ([]byte, error) {
	const (
		saltSize   = 16
		nonceSize  = 12
		iterations = 100_000
		keySize    = 32
	)
	if len(data) < saltSize+nonceSize+16 {
		return nil, errors.New("旧版密文过短")
	}
	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]
	key := pbkdf2.Key(password, salt, iterations, keySize, sha256.New)
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
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
