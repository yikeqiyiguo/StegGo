package common

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// newSHA256Hex 返回数据 SHA256 的十六进制串。
func newSHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// SHA256File 计算文件全局哈希（用于头部规范与篡改检测）。
func SHA256File(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// readLocalTextFile 读取本地文件前若干字节（用于机器指纹采集）。
func readLocalTextFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

// newLocalCommand 构造本地命令（不经过 shell，避免注入；无网络）。
func newLocalCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// trimRegOutput 裁剪 reg.exe 输出为键值。
func trimRegOutput(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		parts := strings.Fields(ln)
		for i := range parts {
			if strings.HasPrefix(parts[i], "REG_") {
				if i+1 < len(parts) {
					return strings.Join(parts[i+1:], " ")
				}
			}
		}
	}
	return ""
}

// EnsureDir 确保目录存在（不存在则创建）。
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// IsExist 判断路径是否存在。
func IsExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir 判断路径是否为目录。
func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// CopyFile 流式复制文件，支持进度回调，避免大文件 OOM。
// 返回复制的字节数。
func CopyFile(src, dst string, progress func(done, total int64)) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	st, err := in.Stat()
	if err != nil {
		return 0, err
	}
	total := st.Size()

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return 0, err
	}
	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	buf := make([]byte, DefaultChunkSize)
	var done int64
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return done, werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return done, rerr
		}
	}
	return done, out.Sync()
}

// ReadAllSafe 读取整个文件（仅用于中小文件；大文件请用流式 API）。
func ReadAllSafe(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WalkFilesByExt 递归收集指定扩展名的文件。
func WalkFilesByExt(root string, exts []string, out *[]string) error {
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		e = strings.ToLower(e)
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extSet[e] = true
	}
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if extSet[strings.ToLower(filepath.Ext(p))] {
			*out = append(*out, p)
		}
		return nil
	})
}

// SafeFileName 规范化文件名，去除路径分隔符与非法字符。
func SafeFileName(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			return '_'
		}
		return r
	}, name)
	return name
}

// HomeDir 返回用户主目录（跨平台）。
func HomeDir() string {
	if runtime.GOOS == "windows" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// AppDataDir 返回应用数据目录（配置文件/审计日志存放处）。
func AppDataDir() (string, error) {
	base := ""
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(HomeDir(), "AppData", "Roaming")
		}
		base = filepath.Join(base, "StegGo")
	case "darwin":
		base = filepath.Join(HomeDir(), "Library", "Application Support", "StegGo")
	default:
		base = filepath.Join(HomeDir(), ".config", "steggo")
	}
	if err := EnsureDir(base); err != nil {
		return "", err
	}
	return base, nil
}

// ErrUnsupported 表示功能未启用/不可用（优雅降级，不崩溃）。
var ErrUnsupported = errors.New("该功能在当前构建中未启用")
