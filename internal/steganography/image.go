package steganography

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
)

var jpegTail = []byte("STGJPEG0")

// readFileBytes reads a whole file into memory using io.Copy.
func readFileBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EmbedPNG hides payload inside a PNG image using LSB steganography.
// 1 bit from each of R, G, B channels per pixel (3 bits/pixel).
// Bit-stream layout: [8 bytes uint64 payload_len][payload bytes]
func EmbedPNG(carrierPath, outputPath string, payload []byte) error {
	f, err := os.Open(carrierPath)
	if err != nil {
		return err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	w, h := bounds.Dx(), bounds.Dy()
	capacity := (w*h*3)/8 - 8
	if len(payload) > capacity {
		return fmt.Errorf("carrier PNG too small: capacity %d B, need %d B", capacity, len(payload))
	}

	hdr := make([]byte, 8)
	binary.BigEndian.PutUint64(hdr, uint64(len(payload)))
	stream := append(hdr, payload...)
	bits := bytesToBits(stream)

	bi := 0
outer:
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if bi >= len(bits) {
				break outer
			}
			c := dst.NRGBAAt(x, y)
			if bi < len(bits) {
				c.R = (c.R & 0xFE) | bits[bi]
				bi++
			}
			if bi < len(bits) {
				c.G = (c.G & 0xFE) | bits[bi]
				bi++
			}
			if bi < len(bits) {
				c.B = (c.B & 0xFE) | bits[bi]
				bi++
			}
			dst.SetNRGBA(x, y, c)
		}
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, dst)
}

// ExtractPNG recovers a payload hidden by EmbedPNG.
func ExtractPNG(carrierPath string) ([]byte, error) {
	f, err := os.Open(carrierPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	rawBits := make([]uint8, 0, bounds.Dx()*bounds.Dy()*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := dst.NRGBAAt(x, y)
			rawBits = append(rawBits, c.R&1, c.G&1, c.B&1)
		}
	}

	if len(rawBits) < 64 {
		return nil, errors.New("image too small to contain payload")
	}

	lenBytes := bitsToBytes(rawBits[:64])
	payloadLen := int(binary.BigEndian.Uint64(lenBytes))

	const maxPayload = 100 * 1024 * 1024
	if payloadLen <= 0 || payloadLen > maxPayload {
		return nil, errors.New("no valid StegGo payload found (bad length or wrong password)")
	}

	needed := 64 + payloadLen*8
	if needed > len(rawBits) {
		return nil, errors.New("payload length exceeds image capacity")
	}
	return bitsToBytes(rawBits[64:needed]), nil
}

// EmbedJPEG appends payload after a JPEG file's bytes.
// Layout: [original JPEG][payload][payload_len uint64 BE 8B][jpegTail 8B]
func EmbedJPEG(carrierPath, outputPath string, payload []byte) error {
	carrier, err := readFileBytes(carrierPath)
	if err != nil {
		return err
	}
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	out := make([]byte, 0, len(carrier)+len(payload)+16)
	out = append(out, carrier...)
	out = append(out, payload...)
	out = append(out, lenBuf[:]...)
	out = append(out, jpegTail...)
	return os.WriteFile(outputPath, out, 0644)
}

// ExtractJPEG extracts a payload from a JPEG produced by EmbedJPEG.
func ExtractJPEG(carrierPath string) ([]byte, error) {
	data, err := readFileBytes(carrierPath)
	if err != nil {
		return nil, err
	}
	tailStart := len(data) - len(jpegTail)
	if tailStart < 0 || !bytes.Equal(data[tailStart:], jpegTail) {
		return nil, errors.New("no StegGo payload found in JPEG file")
	}
	lenStart := tailStart - 8
	if lenStart < 0 {
		return nil, errors.New("corrupted JPEG payload header")
	}
	payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
	payloadStart := lenStart - payloadLen
	if payloadStart < 0 {
		return nil, errors.New("corrupted JPEG payload length")
	}
	result := make([]byte, payloadLen)
	copy(result, data[payloadStart:lenStart])
	return result, nil
}

// bytesToBits converts bytes to MSB-first bit slice (each element is 0 or 1).
func bytesToBits(data []byte) []uint8 {
	bits := make([]uint8, len(data)*8)
	for i, b := range data {
		for j := 0; j < 8; j++ {
			bits[i*8+j] = (b >> uint(7-j)) & 1
		}
	}
	return bits
}

// bitsToBytes converts an MSB-first bit slice back to bytes.
func bitsToBytes(bits []uint8) []byte {
	out := make([]byte, len(bits)/8)
	for i := range out {
		for j := 0; j < 8; j++ {
			out[i] = (out[i] << 1) | bits[i*8+j]
		}
	}
	return out
}
