package steganography

import (
	"encoding/binary"
	"errors"
)

// magic is the 8-byte marker identifying StegGo payloads.
var magic = []byte("STEGGO01")

const magicLen = 8

// Encode builds a binary payload blob from a filename and AES-encrypted data.
//
// Layout:
//
//	[magic 8B][name_len uint16 BE 2B][name nB][data_len uint32 BE 4B][enc_data dB]
func Encode(filename string, encData []byte) []byte {
	nameb := []byte(filename)
	buf := make([]byte, 0, magicLen+2+len(nameb)+4+len(encData))
	buf = append(buf, magic...)
	buf = append(buf, byte(len(nameb)>>8), byte(len(nameb)))
	buf = append(buf, nameb...)
	dl := uint32(len(encData))
	buf = append(buf, byte(dl>>24), byte(dl>>16), byte(dl>>8), byte(dl))
	buf = append(buf, encData...)
	return buf
}

// Decode parses a binary payload produced by Encode.
func Decode(buf []byte) (filename string, encData []byte, err error) {
	if len(buf) < magicLen+2+4 {
		return "", nil, errors.New("payload too short")
	}
	for i, b := range magic {
		if buf[i] != b {
			return "", nil, errors.New("invalid payload magic: not a StegGo carrier")
		}
	}
	off := magicLen
	nameLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
	off += 2
	if len(buf) < off+nameLen+4 {
		return "", nil, errors.New("payload truncated at name")
	}
	filename = string(buf[off : off+nameLen])
	off += nameLen
	dataLen := int(binary.BigEndian.Uint32(buf[off : off+4]))
	off += 4
	if len(buf) < off+dataLen {
		return "", nil, errors.New("payload truncated at data")
	}
	encData = buf[off : off+dataLen]
	return filename, encData, nil
}
