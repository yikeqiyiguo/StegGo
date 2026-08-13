package crypto

import (
	"crypto/rand"
	"errors"
	"io"

	"steggo/internal/common"
)

// KEM 定义后量子密钥封装（KEM）接口，当前为 CRYSTALS-Kyber 预留。
// 接入具体实现（如 cloudflare/circl 的 kyber768）时仅需实现本接口，
// 并将 kyberImpl 指向真实实现，其余链路无需改动。
type KEM interface {
	// GenerateKeyPair 生成 (公钥, 私钥)。
	GenerateKeyPair() (pub, priv []byte, err error)
	// Encapsulate 用公钥封装共享密钥，返回 (封装密文, 共享密钥)。
	Encapsulate(pub []byte) (ct, shared []byte, err error)
	// Decapsulate 用私钥解封装得到共享密钥。
	Decapsulate(priv, ct []byte) (shared []byte, err error)
	// Name 返回算法名（如 kyber768）。
	Name() string
}

// KyberSchemeName 后量子方案名称常量。
const KyberSchemeName = "kyber768"

// kyberImpl 全局后量子实现（默认 nil=未启用）。
var kyberImpl KEM

// RegisterKyber 注册后量子 KEM 实现（由构建标签或插件注入）。
func RegisterKyber(impl KEM) { kyberImpl = impl }

// KyberAvailable 返回后量子加密是否可用。
func KyberAvailable() bool { return kyberImpl != nil }

// NewKyberKEM 返回已注册的后量子 KEM；未注册时返回错误。
func NewKyberKEM() (KEM, error) {
	if kyberImpl == nil {
		return nil, errors.New("CRYSTALS-Kyber 后量子加密未启用（当前构建未内置后量子实现）")
	}
	return kyberImpl, nil
}

// kyberKeySize 后量子密钥长度常量（校验用）。
const (
	KyberPubKeySize  = 1184 // kyber768 公钥
	KyberPrivKeySize = 2400 // kyber768 私钥
	KyberCipherSize  = 1088 // kyber768 封装密文
	KyberSharedSize  = 32
)

// KyberWrap 对 AES-256 密钥做后量子封装。
// 用于"混合加密"场景：随机生成 AES 主密钥，将其封装后随密文保存；
// 解密时先用 Kyber 私钥解封装得到 AES 主密钥。这样即使量子计算机
// 破解对称密钥交换历史，也无法回退解密（前向安全）。
type KyberWrap struct {
	PubKey  []byte // 接收方公钥
	PrivKey []byte // 接收方私钥
}

// GenerateKyberKeyPair 生成后量子密钥对（未启用时返回明确错误）。
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

// WrapKey 用公钥封装 32 字节 AES 密钥，返回 (封装密文)。
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
	// 用共享密钥对 AES 密钥做 XOR 封装（简易 one-time pad）
	out := make([]byte, len(aesKey))
	for i := range aesKey {
		out[i] = aesKey[i] ^ shared[i%len(shared)]
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
	enc := wrapped[KyberCipherSize:]
	shared, err := kem.Decapsulate(w.PrivKey, ct)
	if err != nil {
		return nil, err
	}
	defer common.Wipe(shared)
	key := make([]byte, len(enc))
	for i := range enc {
		key[i] = enc[i] ^ shared[i%len(shared)]
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
