package common

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadUSBToken 模拟 USB 目录读取令牌文件。
func TestReadUSBToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, USBTokenFileName), []byte("usb-token-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := ReadUSBToken(dir)
	if err != nil {
		t.Fatalf("ReadUSBToken: %v", err)
	}
	if string(token) != "usb-token-123" {
		t.Fatalf("令牌内容错误: %s", token)
	}

	// 目录内无令牌文件 → 回退首个非隐藏文件
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "key.dat"), []byte("fallback-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	token2, err := ReadUSBToken(dir2)
	if err != nil {
		t.Fatalf("回退读取失败: %v", err)
	}
	if string(token2) != "fallback-key" {
		t.Fatalf("回退内容错误: %s", token2)
	}

	// 空目录 → 报错
	dir3 := t.TempDir()
	if _, err := ReadUSBToken(dir3); err == nil {
		t.Fatal("空目录应报错")
	}
}

// TestBuildUSBKey 组合令牌 + 序列号。
// 本测试不要求真实 U 盘：序列号为空时行为取决于系统，这里仅验证
// 令牌读取链路与结果稳定性（同一输入应产生相同指纹）。
func TestBuildUSBKeyDeterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, USBTokenFileName), []byte("token-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	k1, err := BuildUSBKey(dir)
	if err != nil {
		// 无 U 盘环境下 BuildUSBKey 可能失败（序列号为空），属预期
		t.Skipf("当前环境无 USB 设备: %v", err)
	}
	k2, err := BuildUSBKey(dir)
	if err != nil {
		t.Fatalf("第二次读取失败: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("USB 密钥应固定 32 字节，实际 %d", len(k1))
	}
	if string(k1) != string(k2) {
		t.Fatal("相同输入应产生相同 USB 密钥指纹")
	}
}
