package common

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditEntry 是一条加密审计日志记录。
type AuditEntry struct {
	Time   string `json:"time"`   // ISO8601 时间
	Action string `json:"action"` // 操作类型：hide/extract/audit/share/watermark...
	Target string `json:"target"` // 目标文件（脱敏后）
	Result string `json:"result"` // ok / fail
	Detail string `json:"detail"` // 结果摘要（不含密钥/密码）
	Hash   string `json:"hash"`   // 载体全局哈希
}

// AuditLogger 将审计日志逐条 AES 加密落地。
type AuditLogger struct {
	mu     sync.Mutex
	path   string
	key    []byte
	file   *os.File
	closed bool
}

// NewAuditLogger 在指定目录创建/追加加密审计日志。
func NewAuditLogger(dir string) (*AuditLogger, error) {
	if err := EnsureDir(dir); err != nil {
		return nil, err
	}
	key, err := loadOrInitKey(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, AuditLogFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &AuditLogger{path: path, key: key, file: f}, nil
}

// Log 写入一条审计记录（AES 加密后 hex 落盘，每行一条）。
func (l *AuditLogger) Log(e AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if e.Time == "" {
		e.Time = time.Now().Format(time.RFC3339)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	enc, err := EncryptBytes(l.key, raw)
	if err != nil {
		return err
	}
	line := hex.EncodeToString(enc) + "\n"
	if _, err := l.file.WriteString(line); err != nil {
		return err
	}
	return l.file.Sync()
}

// ReadAll 解密读取全部审计记录（供加密导出/审计查看）。
func (l *AuditLogger) ReadAll() ([]AuditEntry, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []AuditEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		enc, err := hex.DecodeString(line)
		if err != nil {
			continue
		}
		raw, err := DecryptBytes(l.key, enc)
		if err != nil {
			continue
		}
		var e AuditEntry
		if json.Unmarshal(raw, &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Close 关闭日志文件。
func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
