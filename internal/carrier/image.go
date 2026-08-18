package carrier

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // 注册 JPEG 解码器：特征点锚定/DCT 可从社交平台重压缩后的 JPEG 提取
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"steggo/internal/algorithm"
)

// imageCarrier 图像载体：委托 internal/algorithm 完成位级嵌入。
//
// 支持格式：PNG / BMP / TIFF（均无损）。
// 载荷为任意字节流；嵌入前由 service 层完成加密与封装。
type imageCarrier struct{}

func (c *imageCarrier) Kind() Kind { return KindImage }

func (c *imageCarrier) Extensions() []string {
	return []string{".png", ".bmp", ".tif", ".tiff"}
}

// Capacity 返回最大可嵌入载荷字节数（bits/8）。
func (c *imageCarrier) Capacity(path string, opt Options) (int64, error) {
	img, err := LoadImage(path)
	if err != nil {
		return 0, err
	}
	alg := algorithm.Get(opt.Algorithm)
	if alg == nil {
		return 0, fmt.Errorf("未知算法: %s", opt.Algorithm)
	}
	bits := alg.Capacity(img, opt.AlgorithmOptions())
	return int64(bits / 8), nil
}

// HasCapacity 判断载体是否足以容纳 size 字节。
func (c *imageCarrier) HasCapacity(path string, size int64, opt Options) (bool, error) {
	cap, err := c.Capacity(path, opt)
	if err != nil {
		return false, err
	}
	return size <= cap, nil
}

// Embed 将载荷嵌入图像并保存（保持原格式）。
func (c *imageCarrier) Embed(path, outPath string, payload []byte, opt Options) error {
	if err := opt.fillDefaults(); err != nil {
		return err
	}
	img, err := LoadImage(path)
	if err != nil {
		return err
	}
	alg := algorithm.Get(opt.Algorithm)
	if alg == nil {
		return fmt.Errorf("未知算法: %s", opt.Algorithm)
	}
	bits := algorithm.ByteToBits(payload)
	capBits := alg.Capacity(img, opt.AlgorithmOptions())
	if len(bits) > capBits {
		return fmt.Errorf("%w: 需要 %d 字节, 可用 %d 字节", ErrTooLarge, len(payload), capBits/8)
	}
	if err := alg.Embed(img, bits, opt.AlgorithmOptions()); err != nil {
		return err
	}
	return SaveImage(img, outPath)
}

// Extract 从图像提取位流并还原为字节。
// 注意：返回长度 = 图像容量字节数，载荷真实长度由上层通过头部魔数解析。
func (c *imageCarrier) Extract(path string, opt Options) ([]byte, error) {
	if err := opt.fillDefaults(); err != nil {
		return nil, err
	}
	img, err := LoadImage(path)
	if err != nil {
		return nil, err
	}
	alg := algorithm.Get(opt.Algorithm)
	if alg == nil {
		return nil, fmt.Errorf("未知算法: %s", opt.Algorithm)
	}
	bits, err := alg.Extract(img, opt.AlgorithmOptions())
	if err != nil {
		return nil, err
	}
	return algorithm.BitsToBytes(bits), nil
}

// ---------------------------------------------------------------------------
// 图像读写
// ---------------------------------------------------------------------------

// DecodeImageBytes 从内存字节解码图像为 NRGBA（供 WASM 前端等无文件场景使用）。
func DecodeImageBytes(data []byte) (*image.NRGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return toNRGBA(img), nil
}

// LoadImage 解码任意支持的图像（PNG/BMP/TIFF/JPEG 头会被拦截于 DetectKind）。
func LoadImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("图像解码失败: %w", err)
	}
	return toNRGBA(src), nil
}

// toNRGBA 将任意图像统一转换为 NRGBA（保持像素值精确，避免 alpha 预乘失真）。
func toNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	// 源已是 NRGBA 时直接逐像素复制，避免 .RGBA() 做 alpha 预乘导致
	// 半透明像素 RGB 值改变（这是 GUI 中部分 PNG 嵌入后无法提取的根因）。
	if s, ok := src.(*image.NRGBA); ok {
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				dst.SetNRGBA(x, y, s.NRGBAAt(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	}
	// 源是预乘 RGBA 时解预乘得到精确 NRGBA。
	if s, ok := src.(*image.RGBA); ok {
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				dst.SetNRGBA(x, y, color.NRGBAModel.Convert(s.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
			}
		}
		return dst
	}
	// 其他类型（Gray/Gray16/Paletted 等）经 .RGBA() 归一化转换。
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(bl >> 8),
				A: uint8(a >> 8),
			})
		}
	}
	return dst
}

// SaveImage 依据输出文件扩展名编码保存图像（PNG/BMP/TIFF）。
func SaveImage(img *image.NRGBA, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(outPath)) {
	case ".bmp":
		return bmp.Encode(f, img)
	case ".tif", ".tiff":
		return tiff.Encode(f, img, nil)
	default: // .png 及其他
		return png.Encode(f, img)
	}
}
