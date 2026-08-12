package steg

import (
	"encoding/binary"
	"errors"
	"fmt"

	"steggo/pkg/crypto"
)

// =============================================================
// V2 载体数据格式（自研抗检测 LSB 写入图片时的位流布局）
//
//	[magic 8B]        "STEGGO2A"
//	[version 1B]      2
//	[flags 1B]        bit0=ZIP压缩 bit1=目录打包
//	[bitDepth 1B]     1/2/3/4
//	[nameLen 2B BE]
//	[name nameLen B]
//	[salt 16B]
//	[nonce 12B]
//	[cipherLen 4B BE]
//	[cipherSHA256 32B]
//	[密文 cipherLen B]
//
// 提取流程：SHA256 绑定校验 → PBKDF2 派生 → AES-GCM 解密 → (可选)解 ZIP。
// =============================================================

// MagicV2 是新版隐写载体的固定标识。
var MagicV2 = []byte("STEGGO2A")

const (
	versionV2        = 2
	flagZIP    byte  = 1 << 0
	flagDir    byte  = 1 << 1
	flagStream byte  = 1 << 2
	headerFixedBytes = 8 + 1 + 1 + 1 + 2 + 16 + 12 + 4 + 32 // 77
)

// Meta 描述一次隐写的数据元信息。
type Meta struct {
	Name     string // 原始文件名（目录打包时为根目录名）
	IsZIP    bool   // 密文是否 ZIP 压缩
	IsDir    bool   // 是否为目录打包
	BitDepth int    // 嵌入位数
	Size     int64  // 明文原始大小
}

// Header 是解析出的载体头部。
type Header struct {
	Version   byte
	Flags     byte
	BitDepth  int
	Name      string
	Salt      []byte
	Nonce     []byte
	CipherLen int
	CipherSum [32]byte
}

// Payload 是嵌入载体的完整数据（头部 + 密文）。
type Payload struct {
	Header *Header
	Cipher []byte
}

// EncodeHeader 序列化头部。
func EncodeHeader(h *Header) []byte {
	enc := h.Flags
	nameb := []byte(h.Name)
	buf := make([]byte, 0, headerFixedBytes+len(nameb))
	buf = append(buf, MagicV2...)
	buf = append(buf, versionV2, enc, byte(h.BitDepth))
	buf = append(buf, byte(len(nameb)>>8), byte(len(nameb)))
	buf = append(buf, nameb...)
	buf = append(buf, h.Salt...)
	buf = append(buf, h.Nonce...)
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(h.CipherLen))
	buf = append(buf, lenb[:]...)
	buf = append(buf, h.CipherSum[:]...)
	return buf
}

// ParseHeader 从位流开头解析头部，返回头部长度。
func ParseHeader(stream []byte) (*Header, int, error) {
	if len(stream) < 8 {
		return nil, 0, errors.New("数据过短")
	}
	if string(stream[:8]) != string(MagicV2) {
		return nil, 0, errors.New("非 StegGo V2 载体")
	}
	if len(stream) < headerFixedBytes {
		return nil, 0, errors.New("头部数据不完整")
	}
	off := 8
	h := &Header{}
	h.Version = stream[off]
	off++
	flags := stream[off]
	off++
	if flags&(flagZIP|flagDir) != 0 {
		h.Flags = flags
	}
	h.BitDepth = int(stream[off])
	off++
	if h.BitDepth < 1 || h.BitDepth > 4 {
		return nil, 0, errors.New("不支持的嵌入位数")
	}
	nameLen := int(binary.BigEndian.Uint16(stream[off : off+2]))
	off += 2
	if len(stream) < off+nameLen {
		return nil, 0, errors.New("文件名不完整")
	}
	h.Name = string(stream[off : off+nameLen])
	off += nameLen
	h.Salt = append([]byte(nil), stream[off:off+crypto.SaltSize]...)
	off += crypto.SaltSize
	h.Nonce = append([]byte(nil), stream[off:off+crypto.NonceSize]...)
	off += crypto.NonceSize
	h.CipherLen = int(binary.BigEndian.Uint32(stream[off : off+4]))
	off += 4
	copy(h.CipherSum[:], stream[off:off+32])
	off += 32
	if h.CipherLen <= 0 {
		return nil, 0, errors.New("密文长度非法")
	}
	return h, off, nil
}

// BuildPayload 组装完整位流数据（头部 + 密文）。
func BuildPayload(h *Header, cipher []byte) []byte {
	head := EncodeHeader(h)
	out := make([]byte, 0, len(head)+len(cipher))
	out = append(out, head...)
	out = append(out, cipher...)
	return out
}

// BuildPayloadBytes 是 BuildPayload 的别名，语义更清晰。
func BuildPayloadBytes(h *Header, cipher []byte) []byte { return BuildPayload(h, cipher) }

// ValidateHeader 校验头部版本兼容性。
func ValidateHeader(h *Header) error {
	if h.Version != versionV2 {
		return fmt.Errorf("载体版本不兼容：v%d，仅支持 v%d", h.Version, versionV2)
	}
	return nil
}

// WipeHeader 清理头部中的敏感字段。
func WipeHeader(h *Header) {
	if h == nil {
		return
	}
	crypto.Wipe(h.Salt)
	crypto.Wipe(h.Nonce)
	h.Name = ""
	h.Salt = nil
	h.Nonce = nil
}
