package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// .sg 加密容器：把整个载体文件 AES/SM4 加密打包为独立容器，
// 防止载体被直接篡改、扫描或逆向分析。
//
// 布局：
//
//	[magic 8B]        "STEGGO4C"
//	[version 1B]      1
//	[flags 1B]        bit0=SM4国密（默认 AES-256-GCM）
//	[origLen 8B BE]   明文原始长度（用于完整性校验）
//	[nameLen 2B BE]
//	[name nameLen B]  原始文件名（open 时恢复）
//	[body 密文]       布局与 V1/V2 一致 [salt 16][nonce 12][ciphertext+tag]
const (
	// MagicContainer .sg 加密容器魔数
	MagicContainer = "STEGGO4C"
	// ContainerVersion .sg 容器版本
	ContainerVersion byte = 1
	// ContainerExt .sg 容器扩展名
	ContainerExt = ".sg"

	flagContainerSM4 byte = 1 << 0

	// containerFixed 固定头部字节数（不含文件名）
	containerFixed = 8 + 1 + 1 + 8 + 2
)

// ContainerMeta 描述 .sg 容器的元信息。
type ContainerMeta struct {
	Name   string // 原始文件名
	Size   int64  // 明文大小
	UseSM4 bool   // 是否 SM4 国密加密
	Sum    [32]byte
}

// EncryptContainer 将整个文件内容加密打包为 .sg 容器。
func EncryptContainer(src []byte, name string, password []byte, useSM4 bool) ([]byte, *ContainerMeta, error) {
	if len(src) == 0 {
		return nil, nil, errors.New("容器内容为空")
	}
	if len(password) == 0 {
		return nil, nil, errors.New("容器密码不能为空")
	}
	if name == "" {
		name = "container.bin"
	}
	name = strings.ReplaceAll(name, "\x00", "_")
	if len(name) > 0xFFFF {
		return nil, nil, errors.New("文件名过长")
	}

	body, err := encryptBody(password, useSM4, src)
	if err != nil {
		return nil, nil, err
	}

	nameb := []byte(name)
	flags := byte(0)
	if useSM4 {
		flags |= flagContainerSM4
	}
	head := make([]byte, 0, containerFixed+len(nameb)+len(body))
	head = append(head, MagicContainer...)
	head = append(head, ContainerVersion, flags)
	var lb [8]byte
	binary.BigEndian.PutUint64(lb[:], uint64(len(src)))
	head = append(head, lb[:]...)
	head = append(head, byte(len(nameb)>>8), byte(len(nameb)))
	head = append(head, nameb...)
	head = append(head, body...)

	return head, &ContainerMeta{
		Name:   name,
		Size:   int64(len(src)),
		UseSM4: useSM4,
		Sum:    sha256.Sum256(head),
	}, nil
}

// DecryptContainer 解密 .sg 容器，返回明文与原始文件名。
func DecryptContainer(data []byte, password []byte) ([]byte, *ContainerMeta, error) {
	if len(data) < containerFixed {
		return nil, nil, errors.New("容器数据过短")
	}
	if string(data[:8]) != MagicContainer {
		return nil, nil, errors.New("非 StegGo .sg 加密容器")
	}
	if data[8] != ContainerVersion {
		return nil, nil, fmt.Errorf("不支持的容器版本 %d", data[8])
	}
	flags := data[9]
	origLen := binary.BigEndian.Uint64(data[10:18])
	nameLen := int(binary.BigEndian.Uint16(data[18:20]))
	if len(data) < containerFixed+nameLen {
		return nil, nil, errors.New("容器文件名不完整")
	}
	name := string(data[20 : 20+nameLen])
	body := data[20+nameLen:]

	plaintext, err := decryptBody(password, flags&flagContainerSM4 != 0, body)
	if err != nil {
		return nil, nil, err
	}
	if uint64(len(plaintext)) != origLen {
		return nil, nil, errors.New("容器内容长度校验失败，文件可能已损坏")
	}
	meta := &ContainerMeta{
		Name:   name,
		Size:   int64(len(plaintext)),
		UseSM4: flags&flagContainerSM4 != 0,
	}
	sum := sha256.Sum256(data)
	meta.Sum = sum
	return plaintext, meta, nil
}

// IsContainer 判断字节流是否为 .sg 容器。
func IsContainer(data []byte) bool {
	return len(data) >= len(MagicContainer) && string(data[:len(MagicContainer)]) == MagicContainer
}
