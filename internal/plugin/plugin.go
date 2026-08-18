// Package plugin 提供 StegGo 的基础插件加载框架。
//
// 设计目标：统一管理所有可扩展能力（算法 / 载体 / 加密 / 后量子 KEM /
// 容错编码 / 预设模板 / 工具），供 CLI（plugin list）、审计与第三方
// 集成发现与校验，实现"接口化、彻底解耦"：
//
//   - 业务包无需依赖本包（零耦合）；本包只做注册中心的数据结构与索引。
//   - 内置插件由 CLI 层统一登记；第三方代码可随时 Register 扩展。
//   - 查询按 Kind 分类，便于 UI 分组展示与按类别迭代。
package plugin

import (
	"sort"
	"sync"
)

// Kind 插件类别。
type Kind string

// 插件类别常量。
const (
	KindAlgorithm Kind = "algorithm" // 隐写算法（LSB/DCT/DWT/HUGO/WOW/UNIWARD）
	KindCarrier   Kind = "carrier"   // 载体类型（图像/尾部/零宽等）
	KindCrypto    Kind = "crypto"    // 对称加密（AES-256-GCM/SM4-GCM）
	KindKEM       Kind = "kem"       // 后量子 KEM（ML-KEM-768）
	KindECC       Kind = "ecc"       // 容错编码（Reed-Solomon）
	KindPreset    Kind = "preset"    // 算法参数预设（secrecy/balance/quality）
	KindTool      Kind = "tool"      // 安全工具（三因子/可否认/Shamir/水印/USB）
)

// Info 插件元数据（值对象，注册后只读）。
type Info struct {
	Name        string // 唯一名称
	Kind        Kind   // 类别
	Version     string // 插件版本
	Description string // 功能描述
}

// Registry 插件注册中心（并发安全）。
type Registry struct {
	mu    sync.RWMutex
	items []Info
}

// Register 注册一个插件（同名字段覆盖旧条目）。
func (r *Registry) Register(info Info) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.items {
		if r.items[i].Name == info.Name {
			r.items[i] = info
			return
		}
	}
	r.items = append(r.items, info)
}

// List 返回全部插件（按名称排序）。
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, len(r.items))
	copy(out, r.items)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListByKind 返回指定类别的插件（按名称排序）。
func (r *Registry) ListByKind(kind Kind) []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Info
	for _, it := range r.items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Count 返回已注册插件总数。
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// Default 全局默认注册中心。
var Default = &Registry{}

// Register 快捷注册到全局注册中心。
func Register(info Info) { Default.Register(info) }

// List 快捷查询全局注册中心。
func List() []Info { return Default.List() }

// ListByKind 快捷查询全局注册中心。
func ListByKind(kind Kind) []Info { return Default.ListByKind(kind) }

// Count 快捷查询全局注册中心。
func Count() int { return Default.Count() }
