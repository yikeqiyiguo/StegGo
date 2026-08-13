package carrier

import (
	"bytes"
	"encoding/binary"
	"os"
)

// tailCarrier 尾部容器：将载荷追加到载体文件末尾。
//
// 布局（与 V1.0 internal/steganography 完全一致，向后兼容）：
//
//	[载体原始字节][payload][payload_len uint64 BE 8B][TailTag]
//
// 提取时从文件尾部向前定位 Tag，读取长度并截取 payload。
// 适用于无需重编码、解析器容忍尾部附加数据的格式：WAV/FLAC/PDF/MP4/MKV 等。
type tailCarrier struct {
	kind Kind
	exts []string
}

func (c *tailCarrier) Kind() Kind { return c.kind }

func (c *tailCarrier) Extensions() []string { return c.exts }

// Capacity 尾部容器理论无上限，返回安全阈值防止 OOM。
func (c *tailCarrier) Capacity(path string, opt Options) (int64, error) {
	return maxTailPayload, nil
}

func (c *tailCarrier) HasCapacity(path string, size int64, opt Options) (bool, error) {
	return size <= maxTailPayload && size > 0, nil
}

// Embed 追加 payload 到载体文件末尾。
func (c *tailCarrier) Embed(path, outPath string, payload []byte, opt Options) error {
	if err := opt.fillDefaults(); err != nil {
		return err
	}
	if len(payload) > maxTailPayload {
		return ErrTooLarge
	}
	carrier, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := make([]byte, 0, len(carrier)+len(payload)+8+len(opt.TailTag))
	out = append(out, carrier...)
	out = append(out, payload...)
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	out = append(out, lenBuf[:]...)
	out = append(out, opt.TailTag...)
	return os.WriteFile(outPath, out, 0644)
}

// Extract 从文件尾部提取 payload。
func (c *tailCarrier) Extract(path string, opt Options) ([]byte, error) {
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tag := opt.TailTag
	tailStart := len(data) - len(tag)
	if tailStart < 0 || !bytes.Equal(data[tailStart:], tag) {
		return nil, ErrNoPayload
	}
	lenStart := tailStart - 8
	if lenStart < 0 {
		return nil, ErrCorrupted
	}
	payloadLen := int64(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
	if payloadLen <= 0 || payloadLen > maxTailPayload {
		return nil, ErrCorrupted
	}
	payloadStart := int64(lenStart) - payloadLen
	if payloadStart < 0 {
		return nil, ErrCorrupted
	}
	return data[payloadStart:lenStart], nil
}

// 具体载体注册。
var (
	wavCarrier   = &tailCarrier{kind: KindAudio, exts: []string{".wav", ".flac"}}
	pdfCarrier   = &tailCarrier{kind: KindPDF, exts: []string{".pdf"}}
	videoCarrier = &tailCarrier{kind: KindVideo, exts: []string{".mp4", ".mkv", ".webm", ".avi"}}
)
