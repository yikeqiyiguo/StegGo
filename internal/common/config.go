package common

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是本地加密配置。整个文件以 AES-256-GCM 落盘，不明文保存敏感路径与密钥。
type Config struct {
	// Algorithm 默认隐写算法：lsb / dct / dwt / hugo / wow / uniward
	Algorithm string `json:"algorithm"`
	// BitDepth 默认嵌入位数 1-4
	BitDepth int `json:"bit_depth"`
	// OutputDir 默认输出目录
	OutputDir string `json:"output_dir"`
	// RiskLevel 允许的最低风险等级：SAFE / LOW_RISK / HIGH_RISK
	RiskLevel string `json:"risk_level"`
	// AuditEnabled 是否启用加密审计日志
	AuditEnabled bool `json:"audit_enabled"`
	// AutoRecommend 是否自动推荐算法与 bit 位
	AutoRecommend bool `json:"auto_recommend"`
	// HighDPI 高分屏 DPI 自适应开关
	HighDPI bool `json:"high_dpi"`
	// LastCarrierDir 最近使用的载体目录（加密存储）
	LastCarrierDir string `json:"last_carrier_dir"`
	// LastSecretDir 最近使用的秘密文件目录（加密存储）
	LastSecretDir string `json:"last_secret_dir"`
	// Theme 界面主题
	Theme string `json:"theme"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		Algorithm:     "lsb",
		BitDepth:      2,
		OutputDir:     ".",
		RiskLevel:     RiskLow,
		AuditEnabled:  true,
		AutoRecommend: true,
		HighDPI:       true,
		Theme:         "green",
	}
}

// loadOrInitKey 加载或生成配置目录下的密钥文件。
func loadOrInitKey(dir string) ([]byte, error) {
	return LoadOrCreateKey(filepath.Join(dir, KeyFileName))
}

// LoadConfig 从配置目录加载加密配置；文件不存在时返回默认配置。
func LoadConfig(dir string) (*Config, error) {
	if !IsExist(dir) {
		return DefaultConfig(), nil
	}
	path := filepath.Join(dir, ConfigFileName)
	if !IsExist(path) {
		return DefaultConfig(), nil
	}
	key, err := loadOrInitKey(dir)
	if err != nil {
		return nil, err
	}
	enc, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := DecryptBytes(key, enc)
	if err != nil {
		// 配置被篡改或密钥不匹配：不崩溃，回退默认配置
		return DefaultConfig(), nil
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(raw, cfg); err != nil {
		return DefaultConfig(), nil
	}
	return cfg, nil
}

// SaveConfig 将配置 AES 加密后保存到配置目录。
func SaveConfig(dir string, cfg *Config) error {
	if err := EnsureDir(dir); err != nil {
		return err
	}
	key, err := loadOrInitKey(dir)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	enc, err := EncryptBytes(key, raw)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFileName), enc, 0o600)
}

// ConfigPath 返回配置目录路径（内部辅助）。
func ConfigPath() (string, error) { return AppDataDir() }
