package common

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	Chain  string `json:"chain"`  // 哈希链指纹：SHA256(本条内容 + 上一条指纹)，防篡改
}

// AuditLogger 将审计日志逐条 AES 加密落地。
type AuditLogger struct {
	mu        sync.Mutex
	path      string
	key       []byte
	file      *os.File
	closed    bool
	lastChain string // 上一条记录的哈希链指纹（内存缓存，New 时从文件尾部加载）
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
	al := &AuditLogger{path: path, key: key, file: f}
	al.lastChain = al.loadLastChain()
	return al, nil
}

// Log 写入一条审计记录（AES 加密后 hex 落盘，每行一条）。
// 每条记录通过哈希链与前一条绑定：Chain = SHA256(本条内容 + 上一条 Chain)，
// 任何历史记录被修改都会导致后续链条全部断裂。
func (l *AuditLogger) Log(e AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	if e.Time == "" {
		e.Time = time.Now().Format(time.RFC3339)
	}
	if e.Chain == "" {
		e.Chain = entryDigest(e, l.lastChain)
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
	if err := l.file.Sync(); err != nil {
		return err
	}
	l.lastChain = e.Chain
	return nil
}

// loadLastChain 从日志文件尾部加载最后一条记录的哈希链指纹。
func (l *AuditLogger) loadLastChain() string {
	f, err := os.Open(l.path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	last := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		last = line
	}
	if last == "" {
		return ""
	}
	enc, err := hex.DecodeString(last)
	if err != nil {
		return ""
	}
	raw, err := DecryptBytes(l.key, enc)
	if err != nil {
		return ""
	}
	var e AuditEntry
	if json.Unmarshal(raw, &e) == nil {
		return e.Chain
	}
	return ""
}

// entryDigest 计算单条记录的规范化摘要（排除 Chain 字段，绑定前一条指纹）。
func entryDigest(e AuditEntry, prev string) string {
	payload := struct {
		Time, Action, Target, Result, Detail, Hash string
	}{e.Time, e.Action, e.Target, e.Result, e.Detail, e.Hash}
	raw, _ := json.Marshal(payload)
	h := sha256.New()
	h.Write(raw)
	h.Write([]byte{0x00})
	h.Write([]byte(prev))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyChain 重放校验整条哈希链。任何一条记录被修改，后续链条全部断裂。
// 旧版（无 chain 字段）记录自动跳过；全部无 chain 时返回旧版提示。
func VerifyChain(entries []AuditEntry) error {
	prev := ""
	started := false
	for i, e := range entries {
		if e.Chain == "" {
			continue
		}
		if e.Chain != entryDigest(e, prev) {
			return fmt.Errorf("第 %d 条记录哈希链校验失败：日志已被篡改", i+1)
		}
		prev = e.Chain
		started = true
	}
	if !started {
		return fmt.Errorf("日志为旧版格式，不含哈希链信息（无法校验）")
	}
	return nil
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
