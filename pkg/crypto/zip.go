package crypto

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ZipBytes 将单个文件内容压缩为 zip 字节流。
// 返回的 zip 内仅包含一个名称为 name 的文件条目。
func ZipBytes(data []byte, name string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, err := zw.Create(name)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ZipDir 将整个目录打包为 zip 字节流，完整保留相对目录结构。
func ZipDir(dirPath string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dirPath {
			return nil
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			_, err = zw.Create(rel + "/")
			return err
		}
		fw, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(fw, f); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnzipBytes 将 zip 字节流解压到 destDir，还原完整目录结构。
// 使用 Zip Slip 防护，拒绝写入目标目录之外的路径。
func UnzipBytes(zipData []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Clean(filepath.Join(destDir, f.Name))
		if !strings.HasPrefix(name, filepath.Clean(destDir)+string(os.PathSeparator)) && name != filepath.Clean(destDir) {
			return fmt.Errorf("非法的压缩包路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(name, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

// UnzipBytesToMemory 将 zip 字节流解压到内存，返回第一个文件条目的内容。
func UnzipBytesToMemory(zipData []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, errors.New("压缩包内没有文件")
}

// UnzipSingleFile 解压单文件 zip，返回 (文件名, 内容)。
func UnzipSingleFile(zipData []byte) (string, []byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", nil, err
		}
		return f.Name, data, nil
	}
	return "", nil, errors.New("压缩包内没有文件")
}

// IsZip 判断数据是否为有效的 zip 文件头（PK\x03\x04）。
func IsZip(data []byte) bool {
	return len(data) > 4 && bytes.Equal(data[:4], []byte{0x50, 0x4B, 0x03, 0x04})
}

// ZipDataMeta 记录一次压缩操作的信息。
type ZipDataMeta struct {
	IsDir  bool   // 是否包含目录结构
	Root   string // 解压后的根目录名（目录打包时使用）
	IsZip  bool   // 是否启用了 ZIP 压缩
	RawLen int    // 压缩前原始长度
}

var errNoZip = errors.New("not a zip")

// MaybeZip 尝试压缩：数据量超过阈值时执行 ZIP，否则原样返回。
// 返回数据与是否压缩标记。
func MaybeZip(data []byte, threshold int) ([]byte, bool, error) {
	if threshold > 0 && len(data) > threshold {
		z, err := ZipBytes(data, "secret.bin")
		if err != nil {
			return nil, false, err
		}
		if len(z) < len(data) {
			return z, true, nil
		}
	}
	return data, false, nil
}
