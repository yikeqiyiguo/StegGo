package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"steggo/internal/plugin"
)

// registerBuiltinPlugins 登记全部内置插件（算法/载体/加密/KEM/ECC/预设/工具）。
// 第三方扩展可在任意位置调用 plugin.Register 追加。
func registerBuiltinPlugins() {
	builtin := []plugin.Info{
		// 算法（7）
		{Name: "lsb", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "最低有效位替换，容量大、速度快"},
		{Name: "dct", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "DCT 频域量化嵌入，抗 JPEG 重压缩"},
		{Name: "dwt", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "DWT 小波频域嵌入，抗压缩且稳健"},
		{Name: "hugo", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "学术级自适应失真最小化嵌入（HUGO）"},
		{Name: "wow", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "学术级加权隐藏方向嵌入（WOW）"},
		{Name: "uniward", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "学术级通用小波相对失真嵌入（UNIWARD）"},
		{Name: "anchored", Kind: plugin.KindAlgorithm, Version: "2.2", Description: "特征点锚定隐写：FAST 角点锚定 + QIM 多票冗余，抗旋转/裁剪/JPEG 重压缩"},
		// 载体（5）
		{Name: "image", Kind: plugin.KindCarrier, Version: "2.2", Description: "PNG/BMP/TIFF 无损位图载体"},
		{Name: "audio", Kind: plugin.KindCarrier, Version: "2.2", Description: "WAV/FLAC 无损音频载体"},
		{Name: "trailing", Kind: plugin.KindCarrier, Version: "2.2", Description: "PDF/MP4/MKV/WEBM 尾部追加容器"},
		{Name: "zerowidth", Kind: plugin.KindCarrier, Version: "2.2", Description: "TXT/MD 零宽字符文本载体"},
		{Name: "polyglot", Kind: plugin.KindCarrier, Version: "2.2", Description: "双格式 Polyglot 文件"},
		// 加密（2）
		{Name: "aes-256-gcm", Kind: plugin.KindCrypto, Version: "2.2", Description: "NIST AES-256-GCM 认证加密（默认）"},
		{Name: "sm4-gcm", Kind: plugin.KindCrypto, Version: "2.2", Description: "国密 SM4-GCM（GB/T 32907-2016）"},
		// 后量子 KEM（1）
		{Name: "ml-kem-768", Kind: plugin.KindKEM, Version: "2.2", Description: "NIST FIPS 203 ML-KEM（Kyber 标准版），混合加密"},
		// 容错编码（1）
		{Name: "reed-solomon", Kind: plugin.KindECC, Version: "2.2", Description: "RS(255,239) 容错编码，抗社交压缩与局部损坏（low/medium/high）"},
		// 预设模板（3）
		{Name: "secrecy", Kind: plugin.KindPreset, Version: "2.2", Description: "保密优先：uniward + 1bit"},
		{Name: "balance", Kind: plugin.KindPreset, Version: "2.2", Description: "平衡：dwt + 2bit"},
		{Name: "quality", Kind: plugin.KindPreset, Version: "2.2", Description: "画质优先：lsb + 1bit"},
		// 工具（6）
		{Name: "three-factor", Kind: plugin.KindTool, Version: "2.2", Description: "密码 + 密钥文件 + 本机指纹组合派生"},
		{Name: "deniable", Kind: plugin.KindTool, Version: "2.2", Description: "可否认胁迫隐写（双密文诱饵区）"},
		{Name: "shamir", Kind: plugin.KindTool, Version: "2.2", Description: "GF(2^8) 门限秘密分片（k,n）"},
		{Name: "watermark", Kind: plugin.KindTool, Version: "2.2", Description: "版权隐形水印（公开可提取）"},
		{Name: "usb-key", Kind: plugin.KindTool, Version: "2.2", Description: "USB 硬件密钥盘绑定（令牌 + 设备序列号）"},
		{Name: "container-sg", Kind: plugin.KindTool, Version: "2.2", Description: ".sg 独立容器（载体级 AES/SM4 加密打包）"},
	}
	for _, it := range builtin {
		plugin.Register(it)
	}
}

// kindName 插件类别的中文名。
func kindName(k plugin.Kind) string {
	switch k {
	case plugin.KindAlgorithm:
		return "隐写算法"
	case plugin.KindCarrier:
		return "载体类型"
	case plugin.KindCrypto:
		return "对称加密"
	case plugin.KindKEM:
		return "后量子KEM"
	case plugin.KindECC:
		return "容错编码"
	case plugin.KindPreset:
		return "预设模板"
	case plugin.KindTool:
		return "安全工具"
	default:
		return string(k)
	}
}

// pluginKindOrder 展示排序。
var pluginKindOrder = []plugin.Kind{
	plugin.KindAlgorithm, plugin.KindCarrier, plugin.KindCrypto,
	plugin.KindKEM, plugin.KindECC, plugin.KindPreset, plugin.KindTool,
}

// newPluginCmd 插件框架命令。
func newPluginCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "plugin",
		Short: "插件加载框架：查看已注册插件（算法/载体/加密/KEM/ECC/预设/工具）",
		Long: `StegGo 基础插件加载框架。
统一登记全部可扩展能力（隐写算法、载体类型、对称加密、后量子 KEM、
容错编码、预设模板、安全工具），供发现、校验与第三方扩展。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, _ := cmd.Flags().GetString("kind")
			quiet, _ := cmd.Flags().GetBool("quiet")
			if kind != "" {
				return printPlugins(plugin.Kind(kind), quiet)
			}
			// 无参数：按类别分组展示全部
			if !quiet {
				fmt.Printf("[+] StegGo 插件注册中心：共 %d 个插件\n\n", plugin.Count())
			}
			for _, k := range pluginKindOrder {
				items := plugin.ListByKind(k)
				if len(items) == 0 {
					continue
				}
				fmt.Printf("  %-8s (%d):\n", kindName(k), len(items))
				for _, it := range items {
					fmt.Printf("    %-16s v%-5s %s\n", it.Name, it.Version, it.Description)
				}
				fmt.Println()
			}
			return nil
		},
	}
	root.Flags().String("kind", "", "按类别过滤: algorithm|carrier|crypto|kem|ecc|preset|tool")
	return root
}

// printPlugins 打印指定类别插件列表。
func printPlugins(kind plugin.Kind, quiet bool) error {
	items := plugin.ListByKind(kind)
	if len(items) == 0 {
		return fmt.Errorf("类别 %q 下没有已注册插件", kind)
	}
	if !quiet {
		fmt.Printf("[+] 插件类别 %s（%s）：%d 个\n", kindName(kind), kind, len(items))
	}
	for _, it := range items {
		fmt.Printf("  %-16s v%-5s %s\n", it.Name, it.Version, it.Description)
	}
	return nil
}

// sortedPluginNames 供 help 概览使用的插件名（不重复）。
func sortedPluginNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, it := range plugin.List() {
		if seen[it.Name] {
			continue
		}
		seen[it.Name] = true
		names = append(names, it.Name)
	}
	sort.Strings(names)
	return names
}
