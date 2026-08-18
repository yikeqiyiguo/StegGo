// Package common 提供 StegGo V2.0 的公共基础能力：
//
//	常量定义、内存清理、文件IO、加密工具、配置存储、审计日志、日志封装。
//
// 本层处于五层架构最底层，不依赖任何上层包，保证零循环依赖。
package common

// 应用与版本信息。
const (
	// AppName 应用名称
	AppName = "StegGo"
	// Version V2.0 版本号
	Version = "V2.0"
	// OfflineOnly 离线铁则：本工具纯本地单机运行，无任何网络请求。
	OfflineOnly = "offline-only"
)

// 载体头部魔数与版本。
const (
	// MagicV1 保留：V1.0 早期版本魔数（兼容只读）。
	MagicV1 = "STEGGO1A"
	// MagicV2 与 V1.0 完全兼容的既有魔数。
	MagicV2 = "STEGGO2A"
	// MagicV3 V2.0 新增魔数：头部携带算法标识与全局哈希。
	MagicV3 = "STEGGO3A"

	// VersionByte 当前载体头版本。
	VersionByte byte = 3
	// MinVersionByte 可读取的最低头部版本（保证 V1.0 兼容）。
	MinVersionByte byte = 2

	// HeaderVersionOffset 版本字节在头部中的偏移（8 字节魔数之后）。
	HeaderVersionOffset = 8
)

// 媒体黑名单：有损压缩音视频禁止作为载体。
// JPG/JPEG、MP3、AAC、OGG 等有损格式会破坏隐写数据，三端统一拦截并告警。
var LossyBlacklist = []string{".jpg", ".jpeg", ".mp3", ".aac", ".ogg", ".m4a", ".wma"}

// 默认文件分块大小（大文件流式处理，避免 OOM）。
const DefaultChunkSize = 1 << 20 // 1 MiB

// 配置相关常量。
const (
	// ConfigFileName 本地加密配置文件名称
	ConfigFileName = "config.bin"
	// AuditLogFileName 加密审计日志文件名称
	AuditLogFileName = "audit.log"
	// KeyFileName 配置文件加解密密钥文件名称
	KeyFileName = "key.bin"
)

// 安全审计风险等级。
const (
	RiskSafe = "SAFE"
	RiskLow  = "LOW_RISK"
	RiskHigh = "HIGH_RISK"
)
