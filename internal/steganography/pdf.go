package steganography

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

var pdfTail = []byte("STGPDF01")

// EmbedPDF appends an encrypted payload after the PDF %%EOF marker.
//
// Appended layout:
//
//	[payload][payload_len uint64 BE 8B][pdfTail 8B]
func EmbedPDF(carrierPath, outputPath string, payload []byte) error {
	carrier, err := os.ReadFile(carrierPath)
	if err != nil {
		return err
	}
	if !bytes.Contains(carrier, []byte("%%EOF")) {
		return errors.New("not a valid PDF (no %%EOF found)")
	}
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	out := make([]byte, 0, len(carrier)+len(payload)+16)
	out = append(out, carrier...)
	out = append(out, payload...)
	out = append(out, lenBuf[:]...)
	out = append(out, pdfTail...)
	return os.WriteFile(outputPath, out, 0644)
}

// ExtractPDF extracts a payload from a file produced by EmbedPDF.
func ExtractPDF(carrierPath string) ([]byte, error) {
	data, err := os.ReadFile(carrierPath)
	if err != nil {
		return nil, err
	}
	tailStart := len(data) - len(pdfTail)
	if tailStart < 0 || !bytes.Equal(data[tailStart:], pdfTail) {
		return nil, errors.New("no StegGo payload found in PDF file")
	}
	lenStart := tailStart - 8
	if lenStart < 0 {
		return nil, errors.New("corrupted PDF payload header")
	}
	payloadLen := int(binary.BigEndian.Uint64(data[lenStart : lenStart+8]))
	payloadStart := lenStart - payloadLen
	if payloadStart < 0 {
		return nil, errors.New("corrupted PDF payload length")
	}
	payload := make([]byte, payloadLen)
	copy(payload, data[payloadStart:lenStart])
	return payload, nil
}
