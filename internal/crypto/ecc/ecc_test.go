package ecc

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	data := make([]byte, 1000)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	for _, lv := range []Level{LevelLow, LevelMedium, LevelHigh} {
		enc, stats, err := Encode(data, lv)
		if err != nil {
			t.Fatalf("encode level=%d: %v", lv, err)
		}
		dec, _, err := Decode(enc, lv)
		if err != nil {
			t.Fatalf("decode level=%d: %v", lv, err)
		}
		if !bytes.Equal(dec, data) {
			t.Fatalf("round trip mismatch level=%d", lv)
		}
		if stats.RedundancyRatio <= 0 {
			t.Fatalf("redundancy ratio should be >0, got %v", stats.RedundancyRatio)
		}
	}
}

func TestCorrectErrors(t *testing.T) {
	data := make([]byte, 500)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	lv := LevelMedium // 16 冗余/块，可纠 8 符号/块
	enc, _, err := Encode(data, lv)
	if err != nil {
		t.Fatal(err)
	}
	// 在每个块中注入 4 个符号错误（在纠错能力内）
	body := enc[headerSize:]
	for i := 0; i < len(body)/255; i++ {
		block := body[i*255 : (i+1)*255]
		for j := 0; j < 4; j++ {
			block[j*7+3] ^= 0x5A
		}
	}
	dec, stats, err := Decode(enc, lv)
	if err != nil {
		t.Fatalf("decode with errors: %v", err)
	}
	if !bytes.Equal(dec, data) {
		t.Fatal("corrected data mismatch")
	}
	if stats.CorrectedErrors == 0 {
		t.Fatal("expected corrected errors > 0")
	}
	if stats.RepairRate != 1.0 {
		t.Fatalf("expected repair rate 1.0, got %v", stats.RepairRate)
	}
}

func TestFrameRepair(t *testing.T) {
	frames := [][]byte{[]byte("frame0 payload"), []byte("frame1 payload"), []byte("frame2 payload")}
	tagged, err := TagFrames(frames)
	if err != nil {
		t.Fatal(err)
	}
	// 破坏中间帧
	tagged[14+9] ^= 0xFF
	data, valid, total, rate, err := RepairFrames(tagged)
	if err != nil {
		t.Fatal(err)
	}
	if valid != 2 || total != 3 {
		t.Fatalf("expected 2/3 valid, got %d/%d", valid, total)
	}
	if rate <= 0 {
		t.Fatal("repair rate should be >0")
	}
	_ = data
}
