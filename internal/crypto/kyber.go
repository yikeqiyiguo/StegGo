// Package crypto 的 Kyber 后量子加密（ML-KEM-768）。
//
// 实现基于 Go 标准库 crypto/mlkem（FIPS 203 / NIST 后量子标准化的 ML-KEM，
// 即 CRYSTALS-Kyber 的正式标准版），完全离线、零外部依赖。
// 采用"混合加密"模式：随机生成 AES-256 主密钥加密明文，
// 主密钥再用接收方 ML-KEM 公钥封装；解密时用私钥解封装得到主密钥。
// 即使量子计算机破解对称密钥交换历史，也无法回退解密（前向安全）。
package crypto

import (
	"crypto/mlkem"
	"crypto/rand"
	"errors"
	"io"

	"steggo/internal/common"
)

// KEM 定义后量子密钥封装（KEM）接口。
// 接入具体实现（如标准库 ML-KEM、或后续替换的其他后量子方案）时
// 仅需实现本接口，并将 kyberImpl 指向真实实现，其余链路无需改动。
type KEM interface {
	// GenerateKeyPair 生成 (公钥, 私钥)。
	GenerateKeyPair() (pub, priv []byte, err error)
	// Encapsulate 用公钥封装共享密钥，返回 (封装密文, 共享密钥)。
	Encapsulate(pub []byte) (ct, shared []byte, err error)
	// Decapsulate 用私钥解封装得到共享密钥。
	Decapsulate(priv, ct []byte) (shared []byte, err error)
	// Name 返回算法名（如 ML-KEM-768）。
	Name() string
}

// KyberSchemeName 后量子方案名称常量。
const KyberSchemeName = "ML-KEM-768"

// mlkemKEM 基于标准库 crypto/mlkem 的 ML-KEM-768 实现。
// 安全级别：ML-KEM-768 为 NIST 第三级（AES-192 同等强度）。
type mlkemKEM struct{}

func (mlkemKEM) GenerateKeyPair() (pub, priv []byte, err error) {
	dk, err := mlkem.GenerateKey768()
	if err != nil {
		return nil, nil, err
	}
	return dk.EncapsulationKey().Bytes(), dk.Bytes(), nil
}

func (mlkemKEM) Encapsulate(pub []byte) (ct, shared []byte, err error) {
	ek, err := mlkem.NewEncapsulationKey768(pub)
	if err != nil {
		return nil, nil, errors.New("ML-KEM 公钥解析失败")
	}
	shared, ct = ek.Encapsulate()
	return ct, shared, nil
}

func (mlkemKEM) Decapsulate(priv, ct []byte) (shared []byte, err error) {
	dk, err := mlkem.NewDecapsulationKey768(priv)
	if err != nil {
		return nil, errors.New("ML-KEM 私钥解析失败")
	}
	return dk.Decapsulate(ct)
}

func (mlkemKEM) Name() string { return KyberSchemeName }

// kyberImpl 全局后量子实现。init 自动注册标准库 ML-KEM，
// 其他实现可通过 RegisterKyber 覆盖（如构建标签或插件注入）。
var kyberImpl KEM = mlkemKEM{}

// RegisterKyber 注册后量子 KEM 实现（由构建标签或插件注入）。
func RegisterKyber(impl KEM) { kyberImpl = impl }

// KyberAvailable 返回后量子加密是否可用。
func KyberAvailable() bool { return kyberImpl != nil }

// NewKyberKEM 返回已注册的后量子 KEM。
func NewKyberKEM() (KEM, error) {
	if kyberImpl == nil {
		return nil, errors.New("后量子 KEM 未注册")
	}
	return kyberImpl, nil
}

// 后量子密钥长度常量（ML-KEM-768）。
const (
	KyberPubKeySize  = 1184 // ML-KEM-768 公钥（EncapsulationKey.Bytes）
	KyberPrivKeySize = 64   // ML-KEM-768 私钥种子（DecapsulationKey.Bytes，Go 标准库格式）
	KyberCipherSize  = 1088 // ML-KEM-768 封装密文
	KyberSharedSize  = 32
)

// KyberWrap 对 AES-256 主密钥做后量子封装。
// 用于"混合加密"场景：随机生成 AES 主密钥，将其封装后随密文保存；
// 解密时先用 Kyber 私钥解封装得到 AES 主密钥。这样即使量子计算机
// 破解对称密钥交换历史，也无法回退解密（前向安全）。
type KyberWrap struct {
	PubKey  []byte // 接收方公钥
	PrivKey []byte // 接收方私钥
}

// GenerateKyberKeyPair 生成后量子密钥对。
func GenerateKyberKeyPair() (*KyberWrap, error) {
	kem, err := NewKyberKEM()
	if err != nil {
		return nil, err
	}
	pub, priv, err := kem.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	return &KyberWrap{PubKey: pub, PrivKey: priv}, nil
}

// WrapKey 用公钥封装 32 字节 AES 密钥，返回封装密文。
// 输出布局：[封装密文 1088B][异或密钥 32B]。
func (w *KyberWrap) WrapKey(aesKey []byte) ([]byte, error) {
	if len(aesKey) != common.ToolKeySize {
		return nil, errors.New("AES 密钥必须为 32 字节")
	}
	kem, err := NewKyberKEM()
	if err != nil {
		return nil, err
	}
	ct, shared, err := kem.Encapsulate(w.PubKey)
	if err != nil {
		return nil, err
	}
	// 用共享密钥对 AES 密钥做 XOR 封装（简易 one-time pad）。
	// 共享密钥 32B == AES 密钥 32B，异或后无信息泄露（KEM 保证共享密钥机密性）。
	out := make([]byte, len(aesKey))
	for i := range aesKey {
		out[i] = aesKey[i] ^ shared[i]
	}
	common.Wipe(shared)
	return append(ct, out...), nil
}

// UnwrapKey 用私钥解封装出 AES 密钥。
func (w *KyberWrap) UnwrapKey(wrapped []byte) ([]byte, error) {
	if len(wrapped) < KyberCipherSize+common.ToolKeySize {
		return nil, errors.New("封装数据长度非法")
	}
	kem, err := NewKyberKEM()
	if err != nil {
		return nil, err
	}
	ct := wrapped[:KyberCipherSize]
	enc := wrapped[KyberCipherSize : KyberCipherSize+common.ToolKeySize]
	shared, err := kem.Decapsulate(w.PrivKey, ct)
	if err != nil {
		return nil, err
	}
	defer common.Wipe(shared)
	key := make([]byte, len(enc))
	for i := range enc {
		key[i] = enc[i] ^ shared[i]
	}
	return key, nil
}

// randomBytes 安全随机数辅助。
func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}
