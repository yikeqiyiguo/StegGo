package service

import (
	"fmt"
	"os"
	"path/filepath"

	"steggo/internal/crypto"
)

// ContainerResult .sg 容器操作结果。
type ContainerResult struct {
	Name    string // 原始文件名
	Size    int64  // 明文大小
	UseSM4  bool   // 是否 SM4 国密加密
	OutPath string
}

// ContainerEncrypt 将载体文件整体 AES/SM4 加密打包为 .sg 容器。
// 防止载体被直接篡改、扫描或逆向分析。
func (s *Service) ContainerEncrypt(inPath, outPath string, password []byte, useSM4 bool) (*ContainerResult, error) {
	if inPath == "" {
		return nil, fmt.Errorf("输入文件不能为空")
	}
	if len(password) == 0 {
		return nil, fmt.Errorf("容器密码不能为空")
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return nil, fmt.Errorf("读取文件: %w", err)
	}
	if outPath == "" {
		outPath = inPath + crypto.ContainerExt
	}
	container, meta, err := crypto.EncryptContainer(data, filepath.Base(inPath), password, useSM4)
	if err != nil {
		return nil, fmt.Errorf("加密打包: %w", err)
	}
	if err := os.WriteFile(outPath, container, 0o600); err != nil {
		return nil, fmt.Errorf("写入容器: %w", err)
	}
	res := &ContainerResult{Name: meta.Name, Size: meta.Size, UseSM4: meta.UseSM4, OutPath: outPath}
	s.audit("container-encrypt", inPath, outPath, &Result{Name: meta.Name, Size: meta.Size}, nil)
	return res, nil
}

// ContainerDecrypt 解密 .sg 容器并还原原始文件。
func (s *Service) ContainerDecrypt(inPath, outPath string, password []byte) (*ContainerResult, error) {
	if inPath == "" {
		return nil, fmt.Errorf("输入容器不能为空")
	}
	if len(password) == 0 {
		return nil, fmt.Errorf("容器密码不能为空")
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return nil, fmt.Errorf("读取容器: %w", err)
	}
	plain, meta, err := crypto.DecryptContainer(data, password)
	if err != nil {
		return nil, fmt.Errorf("解密容器: %w", err)
	}
	if outPath == "" {
		outPath = meta.Name
	}
	if err := os.WriteFile(outPath, plain, 0o600); err != nil {
		return nil, fmt.Errorf("写入文件: %w", err)
	}
	res := &ContainerResult{Name: meta.Name, Size: meta.Size, UseSM4: meta.UseSM4, OutPath: outPath}
	s.audit("container-decrypt", inPath, outPath, &Result{Name: meta.Name, Size: meta.Size}, nil)
	return res, nil
}
