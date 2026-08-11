package steganography

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

var audioTail = []byte("STGAUDI0")

// EmbedAudio appends an encrypted payload to a WAV/MP3/FLAC etc. file.
//
// Appended layout (after original file bytes):
//
//	[payload][payload_len uint64 BE 8B][audioTail 8B]
func EmbedAudio(carrierPath, outputPath string, payload []byte) error {
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
	out = append(out, audioTail...)
	return os.WriteFile(outputPath, out, 0644)
}

// ExtractAudio extracts a payload from a file produced by EmbedAudio.
func ExtractAudio(carrierPath string) ([]byte, error) {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	tailStart := len(data) - len(audioTail)
	if tailStart < 0 || !bytes.Equal(data[tailStart:], audioTail) {
		return nil, errors.New("no StegGo payload found in audio file")
	}
	lenStart := tailStart - 8
	if lenStart < 0 {
		return nil, errors.New("corrupted audio payload header")
	}
	payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
	payloadStart := lenStart - payloadLen
	if payloadStart < 0 {
		return nil, errors.New("corrupted audio payload length")
	}
	payload := make([]byte, payloadLen)
	copy(payload, data[payloadStart:lenStart])
	return payload, nil
}
