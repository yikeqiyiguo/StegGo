package main

import (
	"fmt"

	"steggo/internal/common"
	"steggo/internal/service"
)

// applyUSBKey 从 USB 密钥目录构建密钥材料并注入三因子。
// 令牌内容 + USB 设备序列号共同参与派生，令牌被复制到其它设备无法解锁。
func applyUSBKey(opt *service.Options, usbDir string) error {
	if usbDir == "" {
		return nil
	}
	key, err := common.BuildUSBKey(usbDir)
	if err != nil {
		return fmt.Errorf("USB 密钥解锁失败: %w", err)
	}
	if opt.KeyFile != nil {
		// 同时指定 --keyfile 与 --usb 时，两个因子都参与（三因子外挂 USB 令牌）
		combined := make([]byte, 0, len(opt.KeyFile)+len(key)+1)
		combined = append(combined, opt.KeyFile...)
		combined = append(combined, 0xFC)
		combined = append(combined, key...)
		opt.KeyFile = combined
	} else {
		opt.KeyFile = key
	}
	opt.UseKeyFile = true
	return nil
}
