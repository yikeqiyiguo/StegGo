package carrier

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

// LoadImage 加载无损图片（PNG/BMP/TIFF）为 NRGBA。
// JPG/JPEG 通过文件头检测直接拒绝，不提供任何支持入口。
func LoadImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 检测 JPEG 文件头 FF D8
	head := make([]byte, 3)
	if _, err := f.ReadAt(head, 0); err == nil {
		if head[0] == 0xFF && head[1] == 0xD8 {
			return nil, errors.New("有损压缩格式 (JPG/JPEG) 不支持隐写：压缩会销毁隐写数据，已拦截")
		}
	}

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst, nil
}

// SaveImage 将图像保存为 PNG（无损输出）。
func SaveImage(img *image.NRGBA, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, img)
}

// ImageInfo 图片基本信息。
type ImageInfo struct {
	Width, Height int
	Channels      int
}

// GetImageInfo 获取图片尺寸信息。
func GetImageInfo(path string) (ImageInfo, error) {
	img, err := LoadImage(path)
	if err != nil {
		return ImageInfo{}, err
	}
	b := img.Bounds()
	return ImageInfo{Width: b.Dx(), Height: b.Dy(), Channels: 3}, nil
}

// IsLosslessImage 判断文件是否为支持的无损图片格式。
func IsLosslessImage(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

// SaveBMP 以 BMP 保存（供测试使用）。
func SaveBMP(img *image.NRGBA, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return bmp.Encode(out, img)
}

var _ image.Image = (*image.NRGBA)(nil)
var _ color.Color = color.NRGBA{}
