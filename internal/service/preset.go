package service

import "fmt"

// 预设模式：一键切换保密 / 平衡 / 画质优先。
const (
	// PresetSecrecy 保密优先：抗检测最强（自适应成本加权，1 位嵌入）
	PresetSecrecy = "secrecy"
	// PresetBalance 平衡：容量与抗检测折中（DWT 2 位）
	PresetBalance = "balance"
	// PresetQuality 画质优先：视觉质量最好（LSB 1 位）
	PresetQuality = "quality"
)

// ValidPresets 支持的预设列表。
var ValidPresets = []string{PresetSecrecy, PresetBalance, PresetQuality}

// ApplyPreset 应用预设模板到嵌入选项。
// 返回 (应用说明, error)；preset 为空时不做任何修改。
func ApplyPreset(opt *Options, preset string) (string, error) {
	switch preset {
	case "":
		return "", nil
	case PresetSecrecy:
		opt.Algorithm = "uniward"
		opt.CostStyle = "uniward"
		opt.BitDepth = 1
		return "保密优先：uniward 成本加权 + 1 位嵌入", nil
	case PresetBalance:
		opt.Algorithm = "dwt"
		opt.BitDepth = 2
		opt.Levels = 2
		return "平衡模式：DWT 2 级 + 2 位嵌入", nil
	case PresetQuality:
		opt.Algorithm = "lsb"
		opt.BitDepth = 1
		return "画质优先：LSB 1 位嵌入", nil
	default:
		return "", fmt.Errorf("未知预设 %q（可选 secrecy / balance / quality）", preset)
	}
}
