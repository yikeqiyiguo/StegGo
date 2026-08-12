package carrier

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

// WAV 载体采用尾部数据容器：
//
//	[payload][payloadLen uint64 BE 8B][tail 8B]
//
// 数据位于 WAV RIFF 结构之外，播放器完全忽略，不影响音频播放。
var wavTailV2 = []byte("STGAUDV2")
var wavTailV1 = []byte("STGAUDI0") // 早期版本兼容

// EmbedWAV 将载荷嵌入 WAV 文件。
func EmbedWAV(carrierPath, outputPath string, payload []byte) error {
	carrier, err := os.ReadFile(carrierPath)
	if err != nil {
		return err
	}
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	out := make([]byte, 0, len(carrier)+len(payload)+16)
	out = append(out, carrier...)
	out = append(out, payload...)
	out = append(out, lenBuf[:]...)
	out = append(out, wavTailV2...)
	return os.WriteFile(outputPath, out, 0644)
}

// ExtractWAV 从 WAV 载体提取载荷，兼容 V1 旧格式。
func ExtractWAV(carrierPath string) ([]byte, error) {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	tail := wavTailV2
	idx := len(data) - len(tail)
	if idx < 0 || !bytes.Equal(data[idx:], tail) {
		// 尝试 V1 兼容
		idx = len(data) - len(wavTailV1)
		if idx < 0 || !bytes.Equal(data[idx:], wavTailV1) {
			return nil, errors.New("载体中未找到 StegGo 音频载荷")
		}
		tail = wavTailV1
	}
	lenStart := idx - 8
	if lenStart < 0 {
		return nil, errors.New("音频载荷头部损坏")
	}
	payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
	payloadStart := lenStart - payloadLen
	if payloadStart < 0 {
		return nil, errors.New("音频载荷长度非法")
	}
	out := make([]byte, payloadLen)
	copy(out, data[payloadStart:lenStart])
	return out, nil
}

// IsWAV 简单 RIFF/WAVE 头校验。
func IsWAV(data []byte) bool {
	return len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE"
}
