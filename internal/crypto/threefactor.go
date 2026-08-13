package crypto

import (
	"crypto/sha256"
	"errors"
	"os"

	"steggo/internal/common"
)

// FactorMask 三因子组合掩码。
type FactorMask uint8

// 三因子定义。
const (
	FactorPassword FactorMask = 1 << 0 // 密码字符串
	FactorKeyFile  FactorMask = 1 << 1 // KeyFile 密钥文件
	FactorMachine  FactorMask = 1 << 2 // 本机硬件指纹（CPU+主板哈希）
)

// ThreeFactor 描述三因子验证的参与者。
type ThreeFactor struct {
	Password []byte // 密码因子（可为空，但至少要有一个因子）
	KeyFile  []byte // KeyFile 内容（可为空）
	UseMachine bool // 是否绑定本机硬件指纹
}

// Validate 校验三因子组合是否合法（至少启用一个因子）。
func (t *ThreeFactor) Validate() error {
	if len(t.Password) == 0 && len(t.KeyFile) == 0 && !t.UseMachine {
		return errors.New("三因子验证至少需要一个因子")
	}
	return nil
}

// Mask 返回启用的因子掩码。
func (t *ThreeFactor) Mask() FactorMask {
	var m FactorMask
	if len(t.Password) > 0 {
		m |= FactorPassword
	}
	if len(t.KeyFile) > 0 {
		m |= FactorKeyFile
	}
	if t.UseMachine {
		m |= FactorMachine
	}
	return m
}

// Combine 组合各因子为密钥材料（与 BuildPayload 内部逻辑一致）。
// 返回值使用后必须调用 common.Wipe 清理。
func (t *ThreeFactor) Combine() []byte {
	return composeSecret(t.Password, t.KeyFile, len(t.KeyFile) > 0, t.UseMachine)
}

// KeyFileFromFile 从文件读取 KeyFile 内容（限制最大 256KB，防止恶意大文件拖慢派生）。
func KeyFileFromFile(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > 256*1024 {
		return nil, errors.New("KeyFile 过大（>256KB）")
	}
	return os.ReadFile(path)
}

// MachineFingerprint 返回机器指纹的 SHA256（十六进制）。
func MachineFingerprint() string {
	return common.GetMachineID()
}

// FingerprintHash 计算三因子组合指纹（用于审计记录，不含敏感内容）。
func (t *ThreeFactor) FingerprintHash() string {
	raw := t.Combine()
	defer common.Wipe(raw)
	sum := sha256.Sum256(raw)
	return string(sum[:])
}
