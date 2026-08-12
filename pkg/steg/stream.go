package steg

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"steggo/pkg/carrier"
	"steggo/pkg/crypto"
)

// StreamChunkSize 流式处理块大小（64MB）。
// 任意时刻内存占用仅约一个块大小，适合超大文件。
const StreamChunkSize = 64 * 1024 * 1024

// StreamAuto 判断文件大小是否应自动启用流式处理。
func StreamAuto(size int64) bool {
	return size > 512*1024*1024 // >512MB
}

// StreamEmbedGeneric 将大文件流式加密后嵌入音频/PDF 载体。
//
// 密文区采用分块容器：
//
//	[chunk1Len u32][chunk1: nonce+ct][chunk2Len u32][chunk2...]
//
// 每块独立 AES-GCM 加密，全块绑定统一 SHA256。
// （视频载体本身按固定分片处理，使用普通嵌入即可。）
func StreamEmbedGeneric(carrierPath, outputPath, secretPath string, password []byte, opts Options) (*Result, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	switch kind {
	case carrier.KindAudio, carrier.KindPDF:
	default:
		return nil, errors.New("流式嵌入仅支持音频(WAV)/PDF 载体")
	}

	// ---- 阶段一：流式加密到临时文件，累计 SHA256 ----
	tmp, err := os.CreateTemp("", "steggo-stream-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	salt := make([]byte, crypto.SaltSize)
	if _, err := rand.Read(salt); err != nil {
		tmp.Close()
		return nil, err
	}
	key := crypto.DeriveKey(password, salt, crypto.DefaultIterations)
	defer crypto.Wipe(key)

	sf, err := os.Open(secretPath)
	if err != nil {
		tmp.Close()
		return nil, err
	}

	h := sha256.New()
	buf := make([]byte, StreamChunkSize)
	var cipherLen int64
	for {
		n, rerr := sf.Read(buf)
		if n > 0 {
			enc, cerr := crypto.EncryptChunk(buf[:n], key)
			if cerr != nil {
				sf.Close()
				tmp.Close()
				return nil, cerr
			}
			var lenb [4]byte
			binary.BigEndian.PutUint32(lenb[:], uint32(n))
			if _, werr := tmp.Write(lenb[:]); werr != nil {
				sf.Close()
				tmp.Close()
				return nil, werr
			}
			if _, werr := tmp.Write(enc); werr != nil {
				sf.Close()
				tmp.Close()
				return nil, werr
			}
			h.Write(enc)
			cipherLen += int64(4 + len(enc))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			sf.Close()
			tmp.Close()
			return nil, rerr
		}
	}
	sf.Close()
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if cipherLen == 0 {
		return nil, errors.New("秘密文件为空")
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))

	head := &Header{
		Version:   versionV2,
		Flags:     flagStream,
		BitDepth:  opts.BitDepth,
		Name:      filepath.Base(secretPath),
		Salt:      salt,
		Nonce:     make([]byte, crypto.NonceSize), // 分块模式不使用头 nonce
		CipherLen: int(cipherLen),
		CipherSum: sum,
	}

	// ---- 阶段二：流式组装输出 ----
	if err := assembleStream(carrierPath, outputPath, tmpName, head, kind); err != nil {
		return nil, err
	}
	return &Result{Name: head.Name, RawSize: cipherLen}, nil
}

// assembleStream 复制载体并把 [header+密文] 写入合适位置。
func assembleStream(carrierPath, outputPath, tmpName string, head *Header, kind carrier.Kind) error {
	headerBytes := EncodeHeader(head)

	in, err := os.Open(carrierPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	pf, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	defer pf.Close()

	switch kind {
	case carrier.KindPDF:
		eofIdx, err := scanPDFEOFPos(carrierPath)
		if err != nil {
			return err
		}
		if _, err := io.CopyN(out, in, eofIdx); err != nil {
			return err
		}
		if _, err := out.Write(headerBytes); err != nil {
			return err
		}
		if _, err := io.Copy(out, pf); err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	case carrier.KindAudio:
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		if _, err := out.Write(headerBytes); err != nil {
			return err
		}
		if _, err := io.Copy(out, pf); err != nil {
			return err
		}
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(headerBytes)+head.CipherLen))
		if _, err := out.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := out.Write([]byte("STGAUDV2")); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("不支持的载体类型")
	}
}

// scanPDFEOFPos 流式定位最后一个 %%EOF 的位置。
func scanPDFEOFPos(carrierPath string) (int64, error) {
	f, err := os.Open(carrierPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var carry []byte
	var pos int64
	var eofIdx int64 = -1
	chunk := make([]byte, 1<<20)
	for {
		n, rerr := r.Read(chunk)
		if n > 0 {
			search := append(carry, chunk[:n]...)
			last := -1
			cur := 0
			for {
				idx := lastIndexByteSeq(search[cur:], []byte("%%EOF"))
				if idx < 0 {
					break
				}
				last = cur + idx
				cur += idx + 5
			}
			if last >= 0 {
				eofIdx = pos + int64(last)
			}
			if len(search) > 6 {
				carry = append(carry[:0], search[len(search)-6:]...)
			}
			pos += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return 0, rerr
		}
	}
	if eofIdx < 0 {
		return 0, errors.New("不是有效的 PDF（缺少 %%EOF）")
	}
	return eofIdx, nil
}

func lastIndexByteSeq(data, seq []byte) int {
	if len(seq) == 0 {
		return -1
	}
	for i := len(data) - len(seq); i >= 0; i-- {
		ok := true
		for j := range seq {
			if data[i+j] != seq[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// StreamExtractGeneric 流式提取大文件到输出目录。
// 全程不整块载入密文，内存占用约一个块大小。
func StreamExtractGeneric(carrierPath, outputDir string, password []byte) (*Result, error) {
	if len(password) == 0 {
		return nil, errors.New("密码不能为空")
	}
	kind, err := carrier.DetectKind(carrierPath)
	if err != nil {
		return nil, err
	}
	switch kind {
	case carrier.KindAudio, carrier.KindPDF:
	default:
		return nil, errors.New("流式提取仅支持音频(WAV)/PDF 载体")
	}

	payload, err := openPayloadRange(carrierPath, kind)
	if err != nil {
		return nil, err
	}
	defer payload.Close()

	h, headerLen, err := readHeaderStream(payload)
	if err != nil {
		return nil, err
	}
	defer WipeHeader(h)
	if h.Flags&flagStream == 0 {
		return nil, errors.New("该载体不是流式分块格式，请使用普通提取")
	}
	_ = headerLen

	key := crypto.DeriveKey(password, h.Salt, crypto.DefaultIterations)
	defer crypto.Wipe(key)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	out, err := os.Create(filepath.Join(outputDir, h.Name))
	if err != nil {
		return nil, err
	}
	defer out.Close()

	hsh := sha256.New()
	br := bufio.NewReader(payload)
	remain := h.CipherLen
	var plainLen int64
	for remain > 0 {
		var lenb [4]byte
		if _, err := io.ReadFull(br, lenb[:]); err != nil {
			return nil, errors.New("流式密文块损坏")
		}
		blen := int(binary.BigEndian.Uint32(lenb[:]))
		if blen <= 0 || blen > StreamChunkSize {
			return nil, errors.New("流式密文块长度非法")
		}
		if blen+4 > remain {
			return nil, errors.New("流式密文长度不一致")
		}
		enc := make([]byte, blen+crypto.NonceSize+crypto.TagSize)
		if _, err := io.ReadFull(br, enc); err != nil {
			return nil, errors.New("流式密文块不完整")
		}
		hsh.Write(enc)
		pt, err := crypto.DecryptChunk(enc, key)
		if err != nil {
			return nil, err
		}
		remain -= blen + 4
		if _, err := out.Write(pt); err != nil {
			return nil, err
		}
		plainLen += int64(len(pt))
	}
	if !crypto.ConstantTimeEqual(hsh.Sum(nil), h.CipherSum[:]) {
		return nil, errors.New("载荷完整性校验失败：载体可能已被篡改")
	}
	return &Result{Name: h.Name, RawSize: plainLen}, nil
}

// openPayloadRange 打开文件并 seek 到载荷起始位置（流式，不整载）。
func openPayloadRange(carrierPath string, kind carrier.Kind) (*os.File, error) {
	f, err := os.Open(carrierPath)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := info.Size()
	switch kind {
	case carrier.KindAudio:
		var tail [8]byte
		if size < 24 {
			f.Close()
			return nil, errors.New("音频载体过短")
		}
		if _, err := f.ReadAt(tail[:], size-8); err != nil {
			f.Close()
			return nil, err
		}
		if string(tail[:]) != "STGAUDV2" && string(tail[:]) != "STGAUDI0" {
			f.Close()
			return nil, errors.New("未找到音频载荷")
		}
		var lenBuf [8]byte
		if _, err := f.ReadAt(lenBuf[:], size-16); err != nil {
			f.Close()
			return nil, err
		}
		payloadLen := int64(binary.BigEndian.Uint64(lenBuf[:]))
		start := size - 16 - payloadLen
		if start < 0 {
			f.Close()
			return nil, errors.New("音频载荷长度非法")
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	case carrier.KindPDF:
		eofIdx, err := scanPDFEOFPos(carrierPath)
		if err != nil {
			f.Close()
			return nil, err
		}
		// 在 [0, eofIdx) 内反向找 tag
		var tagIdx int64 = -1
		window := int64(1024 * 1024)
		for start := eofIdx; start > 0 && start > eofIdx-window; start -= 4096 {
			readStart := start - 4096
			if readStart < 0 {
				readStart = 0
			}
			head := make([]byte, 4096)
			n, _ := f.ReadAt(head, readStart)
			region := head[:n]
			for i := len(region) - 8; i >= 0; i-- {
				if string(region[i:i+8]) == "STGPDFV2" {
					tagIdx = readStart + int64(i)
					break
				}
			}
			if tagIdx >= 0 {
				break
			}
		}
		if tagIdx < 0 {
			f.Close()
			return nil, errors.New("未找到 PDF 载荷")
		}
		var lenBuf [8]byte
		if _, err := f.ReadAt(lenBuf[:], tagIdx-8); err != nil {
			f.Close()
			return nil, err
		}
		payloadLen := int64(binary.BigEndian.Uint64(lenBuf[:]))
		start := tagIdx - 8 - payloadLen
		if start < 0 {
			f.Close()
			return nil, errors.New("PDF 载荷长度非法")
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}
	f.Close()
	return nil, errors.New("不支持的载体类型")
}

// readHeaderStream 从流中读取头部：先读固定部分，再按名称长度补齐。
func readHeaderStream(r io.Reader) (*Header, int, error) {
	// magic(8)+version(1)+flags(1)+bitDepth(1)+nameLen(2) = 13B
	fixed := make([]byte, 13)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return nil, 0, errors.New("无法读取载荷头部")
	}
	if string(fixed[:8]) != string(MagicV2) {
		return nil, 0, errors.New("非 StegGo V2 载荷")
	}
	nameLen := int(binary.BigEndian.Uint16(fixed[11:13]))
	restLen := 16 + 12 + 4 + 32 + nameLen // salt+nonce+cipherLen+sha256+name
	rest := make([]byte, restLen)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, 0, errors.New("载荷头部不完整")
	}
	full := append(fixed, rest...)
	h, headerLen, err := ParseHeader(full)
	if err != nil {
		return nil, 0, err
	}
	return h, headerLen, nil
}
