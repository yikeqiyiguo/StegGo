package carrier

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

// =============================================================
// 视频载体：帧分片 + XOR 冗余
//
// 载荷被拆分为 N 个固定大小分片，并额外生成 1 片 XOR 冗余片。
// 即使任意 1 个分片因文件损坏缺失，也可通过 XOR 完全恢复。
// 数据容器追加在视频文件尾部，不破坏正常播放。
//
// 布局：
//
//	[chunk0][chunk1]...[chunkN-1][xorChunk]
//	[chunkCount u32][chunkSize u32][payloadLen u64][tail 8B "STGVIDV2"]
// =============================================================

var videoTailV2 = []byte("STGVIDV2")
var videoTailV1 = []byte("STGVID01") // 早期版本兼容

const DefaultVideoChunkSize = 256 * 1024 // 256KB

// EmbedVideo 将载荷分片嵌入视频载体。
func EmbedVideo(carrierPath, outputPath string, payload []byte, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = DefaultVideoChunkSize
	}
	carrier, err := os.ReadFile(carrierPath)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return errors.New("载荷为空")
	}
	chunks, xorChunk := splitChunks(payload, chunkSize)

	out := make([]byte, 0, len(carrier)+len(payload)+chunkSize+24)
	out = append(out, carrier...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	out = append(out, xorChunk...)

	var hdr [20]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(chunks)))
	binary.BigEndian.PutUint32(hdr[4:8], uint32(chunkSize))
	binary.BigEndian.PutUint64(hdr[8:16], uint64(len(payload)))
	copy(hdr[16:20], videoTailV2)
	out = append(out, hdr[:]...)
	return os.WriteFile(outputPath, out, 0644)
}

func splitChunks(payload []byte, chunkSize int) ([][]byte, []byte) {
	n := (len(payload) + chunkSize - 1) / chunkSize
	chunks := make([][]byte, n)
	xor := make([]byte, chunkSize)
	for i := 0; i < n; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		c := make([]byte, chunkSize)
		copy(c, payload[start:end])
		chunks[i] = c
		for j := 0; j < chunkSize; j++ {
			xor[j] ^= c[j]
		}
	}
	return chunks, xor
}

// ExtractVideo 提取视频载荷，支持任意 1 片缺失的 XOR 恢复。
func ExtractVideo(carrierPath string) ([]byte, error) {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	// 查找 V2 尾部元数据
	idx := bytes.LastIndex(data, videoTailV2)
	tailLen := len(videoTailV2)
	if idx < 0 {
		// V1 兼容
		idx = bytes.LastIndex(data, videoTailV1)
		if idx < 0 {
			return nil, errors.New("载体中未找到 StegGo 视频载荷")
		}
		tailLen = len(videoTailV1)
	}
	metaStart := idx + tailLen - 20
	if metaStart < 0 {
		return nil, errors.New("视频载荷头部损坏")
	}
	chunkCount := int(binary.BigEndian.Uint32(data[metaStart : metaStart+4]))
	chunkSize := int(binary.BigEndian.Uint32(data[metaStart+4 : metaStart+8]))
	payloadLen := int(binary.BigEndian.Uint64(data[metaStart+8 : metaStart+16]))
	if chunkCount <= 0 || chunkSize <= 0 || payloadLen <= 0 {
		return nil, errors.New("视频载荷元数据非法")
	}
	total := chunkCount * chunkSize
	chunksStart := metaStart - total
	if chunksStart < 0 {
		return nil, errors.New("视频载荷数据区缺失")
	}
	blob := data[chunksStart:metaStart]
	available := len(blob)
	have := make([][]byte, chunkCount)
	var xor []byte
	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		if start+chunkSize <= available {
			have[i] = blob[start : start+chunkSize]
		}
	}
	if available >= total {
		xor = blob[total : total+chunkSize]
	} else if available > total-chunkSize {
		xor = blob[total-chunkSize:]
	}
	// 补齐 xor 到 chunkSize
	if xor != nil && len(xor) < chunkSize {
		padded := make([]byte, chunkSize)
		copy(padded, xor)
		xor = padded
	}
	// 用 XOR 恢复缺失分片
	missingIdx := -1
	for i := 0; i < chunkCount; i++ {
		if have[i] == nil {
			if missingIdx >= 0 {
				return nil, errors.New("视频载荷缺失超过 1 个分片，无法恢复")
			}
			missingIdx = i
		}
	}
	if missingIdx >= 0 {
		rec := make([]byte, chunkSize)
		if xor == nil {
			return nil, errors.New("视频载荷 XOR 冗余片缺失")
		}
		for i := 0; i < chunkCount; i++ {
			if i == missingIdx {
				continue
			}
			for j := 0; j < chunkSize; j++ {
				rec[j] ^= have[i][j]
			}
		}
		for j := 0; j < chunkSize; j++ {
			rec[j] ^= xor[j]
		}
		have[missingIdx] = rec
	}

	out := make([]byte, 0, payloadLen)
	for i := 0; i < chunkCount; i++ {
		out = append(out, have[i]...)
		if len(out) >= payloadLen {
			break
		}
	}
	return out[:payloadLen], nil
}
