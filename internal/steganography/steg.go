package steganography

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"steggo/internal/crypto"
	"steggo/internal/hash"
)

// Embed hides the secret file inside the carrier, AES-encrypting with password.
// Outputs the steg file to outputPath and the carrier hash to outputPath+".sha256".
func Embed(carrierPath, outputPath, secretPath, password string) error {
	// Read secret file via Open+io.Copy
	sf, err := os.Open(secretPath)
	if err != nil {
		return fmt.Errorf("open secret: %w", err)
	}
	defer sf.Close()
	var sbuf bytes.Buffer
	if _, err := io.Copy(&sbuf, sf); err != nil {
		return fmt.Errorf("read secret: %w", err)
	}
	secretData := sbuf.Bytes()

	// AES-256-GCM encrypt
	encData, err := crypto.Encrypt(secretData, password)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	// Build steganography payload
	secretName := filepath.Base(secretPath)
	payload := Encode(secretName, encData)

	// Dispatch embed by file extension
	if err := embedByExt(carrierPath, outputPath, payload); err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	// Save hash of the OUTPUT steg file (for later integrity verification)
	stegHash, err := hash.File(outputPath)
	if err != nil {
		return fmt.Errorf("hash steg output: %w", err)
	}
	hashPath := outputPath + ".sha256"
	if err := os.WriteFile(hashPath, []byte(stegHash+"\n"), 0600); err != nil {
		return fmt.Errorf("save hash: %w", err)
	}

	fmt.Printf("[OK] Embedded  : %s -> %s\n", secretName, outputPath)
	fmt.Printf("[OK] Steg hash : %s  -> %s\n", hash.FormatShort(stegHash), hashPath)
	return nil
}

// Extract recovers the hidden file from a steg carrier, decrypting with password.
func Extract(carrierPath, outputDir, password string) error {
	rawPayload, err := extractByExt(carrierPath)
	if err != nil {
		return fmt.Errorf("extract payload: %w", err)
	}

	filename, encData, err := Decode(rawPayload)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	plaintext, err := crypto.Decrypt(encData, password)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	outPath := filepath.Join(outputDir, filename)
	if err := os.WriteFile(outPath, plaintext, 0600); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	fmt.Printf("[OK] Extracted : %s -> %s\n", filename, outPath)
	return nil
}

// VerifyCarrier checks that carrierPath hash matches the .sha256 sidecar file.
func VerifyCarrier(carrierPath, hashFilePath string) error {
	ok, err := hash.Verify(carrierPath, hashFilePath)
	if err != nil {
		return err
	}
	if ok {
		fmt.Printf("[OK]   Hash match : %s is intact\n", carrierPath)
	} else {
		fmt.Printf("[WARN] Hash MISMATCH: %s may have been modified!\n", carrierPath)
	}
	return nil
}

// BatchEmbed embeds secretPath into every supported carrier file in carrierDir.
func BatchEmbed(carrierDir, outputDir, secretPath, password string) error {
	entries, err := os.ReadDir(carrierDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !isSupportedExt(ext) {
			continue
		}
		carrierPath := filepath.Join(carrierDir, name)
		outputPath := filepath.Join(outputDir, name)
		if err := Embed(carrierPath, outputPath, secretPath, password); err != nil {
			fmt.Printf("[ERR] %s: %v\n", name, err)
			continue
		}
		count++
	}
	fmt.Printf("[OK] Batch embed: %d file(s) processed\n", count)
	return nil
}

// BatchExtract extracts hidden files from every steg carrier in carrierDir.
func BatchExtract(carrierDir, outputDir, password string) error {
	entries, err := os.ReadDir(carrierDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !isSupportedExt(ext) {
			continue
		}
		carrierPath := filepath.Join(carrierDir, e.Name())
		if err := Extract(carrierPath, outputDir, password); err != nil {
			fmt.Printf("[ERR] %s: %v\n", e.Name(), err)
			continue
		}
		count++
	}
	fmt.Printf("[OK] Batch extract: %d file(s) processed\n", count)
	return nil
}

func embedByExt(carrierPath, outputPath string, payload []byte) error {
	ext := strings.ToLower(filepath.Ext(carrierPath))
	switch ext {
	case ".png":
		return EmbedPNG(carrierPath, outputPath, payload)
	case ".jpg", ".jpeg":
		return EmbedJPEG(carrierPath, outputPath, payload)
	case ".wav", ".mp3", ".flac", ".ogg", ".aac", ".m4a":
		return EmbedAudio(carrierPath, outputPath, payload)
	case ".pdf":
		return EmbedPDF(carrierPath, outputPath, payload)
	default:
		return EmbedAudio(carrierPath, outputPath, payload)
	}
}

func extractByExt(carrierPath string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(carrierPath))
	switch ext {
	case ".png":
		return ExtractPNG(carrierPath)
	case ".jpg", ".jpeg":
		return ExtractJPEG(carrierPath)
	case ".wav", ".mp3", ".flac", ".ogg", ".aac", ".m4a":
		return ExtractAudio(carrierPath)
	case ".pdf":
		return ExtractPDF(carrierPath)
	default:
		return ExtractAudio(carrierPath)
	}
}

func isSupportedExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".wav", ".mp3", ".flac", ".ogg", ".aac", ".m4a", ".pdf":
		return true
	}
	return false
}
