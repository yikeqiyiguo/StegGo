package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"steggo/internal/steganography"
)

var rootCmd = &cobra.Command{
	Use:   "steggo",
	Short: "StegGo - AES-encrypted steganography toolkit",
	Long: `StegGo hides arbitrary files inside PNG/JPEG images, audio files and PDFs
using AES-256-GCM encryption + steganography so the carrier looks unmodified.

Supported carriers:
  Images : .png  (LSB pixel steganography)
           .jpg / .jpeg  (EOF-append, invisible to viewers)
  Audio  : .wav .mp3 .flac .ogg .aac .m4a  (EOF-append)
  Docs   : .pdf  (EOF-append after %%EOF)`,
}

// ── embed ─────────────────────────────────────────────────────────────────────

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Hide a secret file inside a carrier",
	Example: `  steggo embed -c photo.png  -s secret.txt -p "MyPass123" -o steg.png
  steggo embed -c music.mp3 -s keys.txt   -p "MyPass123" -o out.mp3`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := cmd.Flags().GetString("carrier")
		s, _ := cmd.Flags().GetString("secret")
		p, _ := cmd.Flags().GetString("password")
		o, _ := cmd.Flags().GetString("output")
		if c == "" || s == "" || p == "" || o == "" {
			return fmt.Errorf("--carrier, --secret, --password and --output are required")
		}
		return steganography.Embed(c, o, s, p)
	},
}

// ── extract ───────────────────────────────────────────────────────────────────

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Recover a secret file from a steg carrier",
	Example: `  steggo extract -c steg.png  -p "MyPass123" -o ./out/
  steggo extract -c out.mp3  -p "MyPass123" -o ./out/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := cmd.Flags().GetString("carrier")
		p, _ := cmd.Flags().GetString("password")
		o, _ := cmd.Flags().GetString("output")
		if c == "" || p == "" || o == "" {
			return fmt.Errorf("--carrier, --password and --output are required")
		}
		return steganography.Extract(c, o, p)
	},
}

// ── batch-embed ───────────────────────────────────────────────────────────────

var batchEmbedCmd = &cobra.Command{
	Use:   "batch-embed",
	Short: "Embed a secret file into every carrier in a directory",
	Example: `  steggo batch-embed -d ./photos/ -s secret.txt -p "MyPass123" -o ./out/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := cmd.Flags().GetString("dir")
		s, _ := cmd.Flags().GetString("secret")
		p, _ := cmd.Flags().GetString("password")
		o, _ := cmd.Flags().GetString("output")
		if d == "" || s == "" || p == "" || o == "" {
			return fmt.Errorf("--dir, --secret, --password and --output are required")
		}
		return steganography.BatchEmbed(d, o, s, p)
	},
}

// ── batch-extract ─────────────────────────────────────────────────────────────

var batchExtractCmd = &cobra.Command{
	Use:   "batch-extract",
	Short: "Extract hidden files from every carrier in a directory",
	Example: `  steggo batch-extract -d ./out/ -p "MyPass123" -o ./secrets/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := cmd.Flags().GetString("dir")
		p, _ := cmd.Flags().GetString("password")
		o, _ := cmd.Flags().GetString("output")
		if d == "" || p == "" || o == "" {
			return fmt.Errorf("--dir, --password and --output are required")
		}
		return steganography.BatchExtract(d, o, p)
	},
}

// ── verify ────────────────────────────────────────────────────────────────────

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a carrier file against its .sha256 sidecar",
	Example: `  steggo verify -c steg.png -H steg.png.sha256`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := cmd.Flags().GetString("carrier")
		h, _ := cmd.Flags().GetString("hash-file")
		if c == "" {
			return fmt.Errorf("--carrier is required")
		}
		if h == "" {
			h = c + ".sha256"
		}
		return steganography.VerifyCarrier(c, h)
	},
}

func init() {
	// embed flags
	embedCmd.Flags().StringP("carrier", "c", "", "Carrier file (PNG/JPEG/audio/PDF)")
	embedCmd.Flags().StringP("secret", "s", "", "Secret file to hide")
	embedCmd.Flags().StringP("password", "p", "", "Encryption password")
	embedCmd.Flags().StringP("output", "o", "", "Output steg file path")

	// extract flags
	extractCmd.Flags().StringP("carrier", "c", "", "Steg carrier file")
	extractCmd.Flags().StringP("password", "p", "", "Decryption password")
	extractCmd.Flags().StringP("output", "o", "", "Output directory")

	// batch-embed flags
	batchEmbedCmd.Flags().StringP("dir", "d", "", "Directory of carrier files")
	batchEmbedCmd.Flags().StringP("secret", "s", "", "Secret file to embed")
	batchEmbedCmd.Flags().StringP("password", "p", "", "Encryption password")
	batchEmbedCmd.Flags().StringP("output", "o", "", "Output directory")

	// batch-extract flags
	batchExtractCmd.Flags().StringP("dir", "d", "", "Directory of steg carrier files")
	batchExtractCmd.Flags().StringP("password", "p", "", "Decryption password")
	batchExtractCmd.Flags().StringP("output", "o", "", "Output directory for secrets")

	// verify flags
	verifyCmd.Flags().StringP("carrier", "c", "", "Carrier file to verify")
	verifyCmd.Flags().StringP("hash-file", "H", "", "SHA-256 sidecar file (default: <carrier>.sha256)")

	rootCmd.AddCommand(embedCmd, extractCmd, batchEmbedCmd, batchExtractCmd, verifyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
