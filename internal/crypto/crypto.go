// Package crypto 提供 StegGo V2.0 高强度加密体系：
//
//	标准链路：文件压缩 → PBKDF2 派生 → AES-256-GCM 加密 → SHA256 全局哈希校验 → 写入载体
//	三因子验证：密码字符串 + KeyFile 密钥文件 + 本机硬件指纹，可自由组合
//	可否认胁迫隐写：单载体双独立密文，假密码出诱饵，主密码出真实
//	可选 CRYSTALS-Kyber 后量子开关（接口预留，默认关闭）
//
// 与 V1.0 的密文布局 [salt][nonce][ciphertext+tag] 完全一致，保证 100% 向下兼容。
package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"steggo/internal/common"
	v1crypto "steggo/pkg/crypto"
)

// V3 载体头部布局（本包负责序列化/解析）：
//
//	[magic 8B]        "STEGGO3A"
//	[version 1B]      3
//	[flags 1B]        bit0=ZIP bit1=目录 bit2=流式 bit3=Kyber bit4=可否认 bit5=KeyFile bit6=机器指纹
//	[algoID 1B]       算法标识
//	[bitDepth 1B]     嵌入位数 1-4
//	[nameLen 2B BE]
//	[name nameLen B]
//	[salt 16B]
//	[nonce 12B]
//	[cipherLen 4B BE]
//	[cipherSHA256 32B]
//
// 可否认载荷在头部之后追加双区扩展：[len1 4B][len2 4B][hash1 32B][hash2 32B][cipher1][cipher2]
const (
	// MagicV3 V2.0 载体魔数
	MagicV3 = "STEGGO3A"
	// versionV3 头部版本
	versionV3 byte = 3

	flagZIP       byte = 1 << 0
	flagDir       byte = 1 << 1
	flagStream    byte = 1 << 2
	flagKyber     byte = 1 << 3
	flagDeniable  byte = 1 << 4
	flagKeyFile   byte = 1 << 5
	flagMachine   byte = 1 << 6

	// v3HeaderFixed 头部固定字节数（不含文件名）
	v3HeaderFixed = 8 + 1 + 1 + 1 + 1 + 2 + 16 + 12 + 4 + 32
	// deniableExtLen 可否认扩展头长度
	deniableExtLen = 4 + 4 + 32 + 32

	// CompressThreshold 超过该字节数才尝试 ZIP 压缩
	CompressThreshold = 1024
)

// 算法标识。
const (
	AlgoLSB     byte = 0
	AlgoDCT     byte = 1
	AlgoDWT     byte = 2
	AlgoHUGO    byte = 3
	AlgoWOW     byte = 4
	AlgoUNIWARD byte = 5
)

// AlgoNames 算法标识 → 名称。
var AlgoNames = map[byte]string{
	AlgoLSB: "lsb", AlgoDCT: "dct", AlgoDWT: "dwt",
	AlgoHUGO: "hugo", AlgoWOW: "wow", AlgoUNIWARD: "uniward",
}

// AlgoIDToName 算法 ID 转名称。
func AlgoIDToName(id byte) string {
	if s, ok := AlgoNames[id]; ok {
		return s
	}
	return "lsb"
}

// AlgoNameToID 名称转算法 ID。
func AlgoNameToID(name string) (byte, bool) {
	for id, n := range AlgoNames {
		if n == name {
			return id, true
		}
	}
	return AlgoLSB, false
}

// Meta 描述一次隐写的数据元信息。
type Meta struct {
	Name      string // 原始文件名（目录打包时为根目录名）
	IsZIP     bool   // 密文是否 ZIP 压缩
	IsDir     bool   // 是否为目录打包
	Algorithm string // 算法名称
	BitDepth  int    // 嵌入位数
	Size      int64  // 明文原始大小
	Kyber     bool   // 是否使用后量子加密
	Deniable  bool   // 是否为可否认双密文
}

// Header 是解析出的 V3 头部。
type Header struct {
	Version    byte
	Flags      byte
	Algorithm  byte
	BitDepth   int
	Name       string
	Salt       []byte
	Nonce      []byte
	CipherLen  int
	CipherSum  [32]byte
}

// EncodeV3Header 序列化 V3 头部。
func EncodeV3Header(h *Header) []byte {
	nameb := []byte(h.Name)
	buf := make([]byte, 0, v3HeaderFixed+len(nameb))
	buf = append(buf, MagicV3...)
	buf = append(buf, versionV3, h.Flags, h.Algorithm, byte(h.BitDepth))
	buf = append(buf, byte(len(nameb)>>8), byte(len(nameb)))
	buf = append(buf, nameb...)
	buf = append(buf, h.Salt...)
	buf = append(buf, h.Nonce...)
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(h.CipherLen))
	buf = append(buf, lb[:]...)
	buf = append(buf, h.CipherSum[:]...)
	return buf
}

// ParseV3Header 解析 V3 头部，返回头部长度。
func ParseV3Header(stream []byte) (*Header, int, error) {
	if len(stream) < 8 {
		return nil, 0, errors.New("数据过短")
	}
	if string(stream[:8]) != MagicV3 {
		return nil, 0, errors.New("非 StegGo V3 载体")
	}
	if len(stream) < v3HeaderFixed {
		return nil, 0, errors.New("头部数据不完整")
	}
	off := 8
	h := &Header{}
	h.Version = stream[off]
	off++
	h.Flags = stream[off]
	off++
	h.Algorithm = stream[off]
	off++
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
	h.Salt = append([]byte(nil), stream[off:off+16]...)
	off += 16
	h.Nonce = append([]byte(nil), stream[off:off+12]...)
	off += 12
	h.CipherLen = int(binary.BigEndian.Uint32(stream[off : off+4]))
	off += 4
	copy(h.CipherSum[:], stream[off:off+32])
	off += 32
	if h.CipherLen <= 0 {
		return nil, 0, errors.New("密文长度非法")
	}
	return h, off, nil
}

// BuildOptions 描述一次数据加密处理的参数。
type BuildOptions struct {
	Name      string // 原始文件名
	Algorithm string // 算法名称（写入头部）
	BitDepth  int    // 嵌入位数（写入头部）
	Compress  bool   // 是否启用 ZIP 压缩
	Password  []byte // 密码（调用方负责使用后清理）
	KeyFile   []byte // KeyFile 内容（启用时参与密钥派生）
	UseKeyFile bool  // 是否启用 KeyFile 因子
	UseMachine bool  // 是否绑定本机硬件指纹因子
	Kyber     bool   // 是否启用后量子加密（当前版本未内置，启用会返回明确错误）
	IsDir     bool   // 是否为目录打包
}

// composeSecret 按启用的因子组合密钥材料。
// 因子分隔使用不可打印控制字节，避免拼接碰撞。
func composeSecret(password, keyfile []byte, useKeyFile, useMachine bool) []byte {
	out := make([]byte, 0, len(password)+len(keyfile)+64)
	out = append(out, password...)
	if useKeyFile {
		out = append(out, 0xFF)
		out = append(out, keyfile...)
	}
	if useMachine {
		out = append(out, 0xFE)
		out = append(out, []byte(common.GetMachineID())...)
	}
	return out
}

// BuildPayload 构建 V3 加密载荷：
//
//	可选压缩 → 组合三因子 → PBKDF2 派生 → AES-256-GCM 加密 → SHA256 绑定 → V3 头部。
func BuildPayload(data []byte, opt *BuildOptions) ([]byte, *Meta, error) {
	if opt == nil {
		return nil, nil, errors.New("缺少加密参数")
	}
	if len(opt.Password) == 0 && len(opt.KeyFile) == 0 {
		return nil, nil, errors.New("至少需要密码或密钥文件之一")
	}
	if opt.Kyber {
		return nil, nil, common.ErrUnsupported
	}

	secret := composeSecret(opt.Password, opt.KeyFile, opt.UseKeyFile, opt.UseMachine)
	defer common.Wipe(secret)

	// 1. 可选压缩
	toBe := data
	isZIP := false
	if opt.Compress {
		z, ok, err := v1crypto.MaybeZip(data, CompressThreshold)
		if err != nil {
			return nil, nil, err
		}
		toBe, isZIP = z, ok
	}

	// 2. 加密（V1.0 同布局，保证兼容）
	ciphertext, err := v1crypto.Encrypt(toBe, secret)
	if err != nil {
		return nil, nil, err
	}

	// 3. 组装头部
	flags := byte(0)
	if isZIP {
		flags |= flagZIP
	}
	if opt.IsDir {
		flags |= flagDir
	}
	if opt.UseKeyFile {
		flags |= flagKeyFile
	}
	if opt.UseMachine {
		flags |= flagMachine
	}
	algoID, _ := AlgoNameToID(opt.Algorithm)
	head := EncodeV3Header(&Header{
		Flags:     flags,
		Algorithm: algoID,
		BitDepth:  opt.BitDepth,
		Name:      opt.Name,
		Salt:      ciphertext[:16],
		Nonce:     ciphertext[16:28],
		CipherLen: len(ciphertext),
		CipherSum: sha256.Sum256(ciphertext),
	})

	out := make([]byte, 0, len(head)+len(ciphertext))
	out = append(out, head...)
	out = append(out, ciphertext...)

	meta := &Meta{
		Name:      opt.Name,
		IsZIP:     isZIP,
		IsDir:     opt.IsDir,
		Algorithm: AlgoIDToName(algoID),
		BitDepth:  opt.BitDepth,
		Size:      int64(len(data)),
		Kyber:     false,
	}
	return out, meta, nil
}

// ParseOptions 描述解密参数。
type ParseOptions struct {
	Password  []byte // 密码（调用方负责清理）
	KeyFile   []byte // KeyFile 内容
}

// TrimPayload 从提取流中截取精确载荷。
// 图像载体提取流长度 = 载体容量（载荷后带噪声填充），本函数依据 V3 头部
// 定位载荷边界，返回 头部+扩展头+密文 的完整切片。
func TrimPayload(stream []byte) ([]byte, error) {
	head, headLen, err := ParseV3Header(stream)
	if err != nil {
		return nil, err
	}
	total := headLen + head.CipherLen
	if head.Flags&flagDeniable != 0 {
		total += deniableExtLen
	}
	if len(stream) < total {
		return nil, errors.New("载荷不完整：流长度不足")
	}
	return stream[:total], nil
}

// ParsePayload 解析 V3 载荷并解密。
// 支持 V1.0（STEGGO2A）与 V3（STEGGO3A）两种头部；V1.0 头部算法视为 lsb。
func ParsePayload(payload []byte, opt *ParseOptions) ([]byte, *Meta, error) {
	if len(payload) < 8 {
		return nil, nil, errors.New("载荷过短")
	}
	if opt == nil {
		opt = &ParseOptions{}
	}
	head, headLen, err := ParseV3Header(payload)
	if err != nil {
		// 回退 V1.0 解析（复用 pkg/steg 的兼容逻辑由上层完成）
		return nil, nil, err
	}
	secret := composeSecret(opt.Password, opt.KeyFile, head.Flags&flagKeyFile != 0, head.Flags&flagMachine != 0)
	defer common.Wipe(secret)

	ciphertext := payload[headLen:]
	if len(ciphertext) != head.CipherLen {
		return nil, nil, errors.New("密文长度与头部不一致，载体可能已损坏")
	}
	sum := sha256.Sum256(ciphertext)
	if sum != head.CipherSum {
		return nil, nil, errors.New("全局哈希校验失败：载体已被篡改")
	}
	plaintext, err := v1crypto.Decrypt(ciphertext, secret)
	if err != nil {
		return nil, nil, err
	}
	meta := &Meta{
		Name:      head.Name,
		IsZIP:     head.Flags&flagZIP != 0,
		IsDir:     head.Flags&flagDir != 0,
		Algorithm: AlgoIDToName(head.Algorithm),
		BitDepth:  head.BitDepth,
		Size:      int64(len(plaintext)),
		Kyber:     head.Flags&flagKyber != 0,
		Deniable:  head.Flags&flagDeniable != 0,
	}
	return plaintext, meta, nil
}

// HeaderString 返回头部可读摘要（不含敏感字段）。
func HeaderString(h *Header) string {
	return fmt.Sprintf("v%d algo=%s bits=%d name=%q flags=0x%02x",
		h.Version, AlgoIDToName(h.Algorithm), h.BitDepth, h.Name, h.Flags)
}
