package common

import (
	"runtime"
)

// Wipe 用零值覆盖字节切片，防止密码、密钥、解密临时缓冲区残留内存。
// 通过 runtime.KeepAlive 阻止编译器优化掉覆盖操作。
func Wipe(b []byte) {
	if len(b) == 0 {
		return
	}
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// WipeStrings 覆盖并释放多个字符串（通过转成可变切片覆盖）。
// Go 的 string 不可变，此函数仅作尽力而为的清理，真正安全应使用 []byte 承载密码。
func WipeStrings(ss ...string) {
	for _, s := range ss {
		b := []byte(s)
		for i := range b {
			b[i] = 0
		}
	}
}

// WipeKey 等价于 Wipe，语义更明确（用于密钥）。
func WipeKey(b []byte) { Wipe(b) }

// SecretBytes 是一个携带自动清理语义的字节容器。
// 使用完毕后调用 Close 会主动清零底层数据，防止内存 Dump 泄露。
type SecretBytes struct {
	data []byte
}

// NewSecretBytes 包装一段敏感字节。
func NewSecretBytes(b []byte) *SecretBytes { return &SecretBytes{data: b} }

// Bytes 返回底层字节（调用方需在完成后主动 Close）。
func (s *SecretBytes) Bytes() []byte { return s.data }

// Len 返回字节长度。
func (s *SecretBytes) Len() int { return len(s.data) }

// Close 清零并释放底层数据。
func (s *SecretBytes) Close() {
	if s == nil {
		return
	}
	Wipe(s.data)
	s.data = nil
	runtime.KeepAlive(s)
}

// GetMachineID 返回当前机器指纹（CPU+主板哈希，用于三因子验证）。
// 纯本地读取 /proc/cpuinfo 或 Windows 注册表 BIOS 信息，无任何网络请求。
func GetMachineID() string {
	info := collectMachineInfo()
	return newSHA256Hex(info)
}

// collectMachineInfo 汇总可读的本地硬件信息（无网络）。
func collectMachineInfo() string {
	out := ""
	out += readLocalTextFile("/proc/cpuinfo")
	out += "|"
	out += readLocalTextFile("/proc/device-tree/model")
	out += "|"
	if v, ok := readRegQuery("HKLM\\HARDWARE\\DESCRIPTION\\System\\BIOS", "SystemManufacturer"); ok {
		out += v
	}
	out += "|"
	if v, ok := readRegQuery("HKLM\\HARDWARE\\DESCRIPTION\\System\\BIOS", "SystemProductName"); ok {
		out += v
	}
	return out
}

// readRegQuery 在 Windows 上通过 reg.exe 读取注册表键值（纯本地命令，无网络）。
func readRegQuery(key, name string) (string, bool) {
	cmd := newLocalCommand("reg", "query", key, "/v", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return trimRegOutput(string(out)), true
}
