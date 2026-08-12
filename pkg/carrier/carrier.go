// Package carrier 负责全品类载体文件的解析与读写：
//
//	图片 : PNG / BMP / TIFF（无损格式，JPG/JPEG 直接拦截）
//	音频 : WAV（无损，MP3 等有损直接拦截）
//	文档 : PDF（%%EOF 前插入内部冗余数据流，不破坏渲染结构）
//	文本 : TXT/MD（零宽字符隐写，肉眼完全不可见）
//	视频 : 任意视频（帧分片 + XOR 冗余，尾部数据容器，不破坏播放）
package carrier

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FileSize 返回文件大小（字节）。
func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Carrier 是载体统一接口。
type Carrier interface {
	// Ext 返回文件扩展名
	Ext() string
}

// Kind 载体类别。
type Kind int

const (
	KindUnknown Kind = iota
	KindImage
	KindAudio
	KindPDF
	KindText
	KindVideo
)

// DetectKind 根据扩展名判断载体类别。
func DetectKind(path string) (Kind, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".bmp", ".tif", ".tiff":
		return KindImage, nil
	case ".jpg", ".jpeg":
		return KindUnknown, errors.New("有损压缩格式 (JPG/JPEG) 不支持隐写：压缩会销毁隐写数据，已拦截")
	case ".wav":
		return KindAudio, nil
	case ".mp3", ".flac", ".ogg", ".aac", ".m4a", ".wma":
		return KindUnknown, errors.New("有损/非标音频格式不支持隐写，仅支持无损 WAV")
	case ".pdf":
		return KindPDF, nil
	case ".txt", ".md", ".markdown":
		return KindText, nil
	case ".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv":
		return KindVideo, nil
	}
	return KindUnknown, errors.New("不支持的载体格式: " + ext)
}

// String 返回载体类别名称。
func (k Kind) String() string {
	switch k {
	case KindImage:
		return "图片"
	case KindAudio:
		return "音频"
	case KindPDF:
		return "PDF文档"
	case KindText:
		return "文本"
	case KindVideo:
		return "视频"
	}
	return "未知"
}

// IsSupported 判断扩展名是否支持作为载体。
func IsSupported(path string) bool {
	k, err := DetectKind(path)
	return err == nil && k != KindUnknown
}

// IsTextLike 判断是否为文本类载体。
func IsTextLike(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".markdown":
		return true
	}
	return false
}
