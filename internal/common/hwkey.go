package common

import (
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// USB 密钥绑定解锁：
//
//	--usb <目录> 指定 USB 密钥盘挂载目录。
//	工具读取 USB 内的密钥令牌文件（usb_token.bin 或目录内首个文件）并与
//	该 USB 设备的序列号组合为密钥材料，参与三因子派生。
//	令牌被复制到其它 U 盘时序列号不同，密钥材料不同，无法解锁 → 绑定设备。
const (
	// USBTokenFileName USB 密钥令牌文件名（首选）
	USBTokenFileName = "usb_token.bin"
	// usbFactorSep 令牌与序列号之间的分隔字节（不可打印，防拼接碰撞）
	usbFactorSep = 0xFD
)

// GetUSBDeviceSN 返回可移动磁盘（USB）设备序列号，失败返回空串。
// 纯本地系统查询，无网络请求。
func GetUSBDeviceSN() string {
	switch runtime.GOOS {
	case "windows":
		return winUSBDeviceSN()
	case "linux":
		return linuxUSBDeviceSN()
	case "darwin":
		return darwinUSBDeviceSN()
	default:
		return ""
	}
}

// winUSBDeviceSN Windows：枚举磁盘驱动器，取含 USBSTOR 的序列号。
func winUSBDeviceSN() string {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-PnpDevice -Class DiskDrive -ErrorAction SilentlyContinue | Where-Object { $_.InstanceId -like '*USBSTOR*' } | Select-Object -ExpandProperty InstanceId").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "USBSTOR") {
			// InstanceId 形如 USBSTOR\DISK&VEN_...&PROD_...&REV_1000\A1B2C3D4&0
			parts := strings.Split(line, "\\")
			if len(parts) > 0 {
				id := parts[len(parts)-1]
				if i := strings.Index(id, "&"); i > 0 {
					id = id[:i]
				}
				if id != "" {
					return strings.ToUpper(id)
				}
			}
		}
	}
	return ""
}

// linuxUSBDeviceSN Linux：遍历 /sys/class/block 读取设备序列号。
func linuxUSBDeviceSN() string {
	entries, err := os.ReadDir("/sys/class/block")
	if err != nil {
		return ""
	}
	var seen []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "sd") && !strings.HasPrefix(name, "mmcblk") && !strings.HasPrefix(name, "nvme") {
			continue
		}
		serial := readLocalTextFile(filepath.Join("/sys/class/block", name, "device", "serial"))
		serial = strings.TrimSpace(serial)
		if serial == "" {
			continue
		}
		dup := false
		for _, s := range seen {
			if s == serial {
				dup = true
				break
			}
		}
		if !dup {
			seen = append(seen, serial)
		}
	}
	if len(seen) > 0 {
		return strings.ToUpper(seen[0])
	}
	return ""
}

// darwinUSBDeviceSN macOS：通过 system_profiler 查询（可能较慢，失败返回空）。
func darwinUSBDeviceSN() string {
	out, err := exec.Command("system_profiler", "SPUSBDataType").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Serial Number:") {
			sn := strings.TrimSpace(strings.TrimPrefix(line, "Serial Number:"))
			if sn != "" && !strings.Contains(sn, "N/A") {
				return strings.ToUpper(sn)
			}
		}
	}
	return ""
}

// ReadUSBToken 从 USB 目录读取密钥令牌文件。
// 优先 usb_token.bin，其次目录内第一个文件（排除隐藏/临时文件）。
func ReadUSBToken(dir string) ([]byte, error) {
	if dir == "" {
		return nil, errors.New("USB 目录不能为空")
	}
	primary := filepath.Join(dir, USBTokenFileName)
	if data, err := os.ReadFile(primary); err == nil && len(data) > 0 {
		return data, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New("无法访问 USB 目录: " + err.Error())
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, errors.New("USB 目录中未找到密钥令牌文件（建议命名为 usb_token.bin）")
}

// BuildUSBKey 组合 USB 密钥材料：令牌内容 + 设备序列号。
// 返回的 []byte 作为三因子中的 KeyFile 使用；序列号参与派生后，
// 令牌被复制到其它设备也无法解锁。
func BuildUSBKey(dir string) ([]byte, error) {
	token, err := ReadUSBToken(dir)
	if err != nil {
		return nil, err
	}
	sn := GetUSBDeviceSN()
	if sn == "" {
		return nil, errors.New("未能读取 USB 设备序列号，请确认 U 盘已插入且系统可识别")
	}
	out := make([]byte, 0, len(token)+len(sn)+8)
	out = append(out, token...)
	out = append(out, usbFactorSep)
	out = append(out, sn...)
	h := sha256.Sum256(out)
	// 返回 32 字节指纹，长度固定，避免意外泄露令牌内容长度信息
	return h[:], nil
}
