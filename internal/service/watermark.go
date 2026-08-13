package service

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"

	"steggo/internal/carrier"
)

// watermarkPrefix 水印内容前缀（公开可提取，无需密码）。
const watermarkPrefix = "WMV2:"

// watermarkSeed 水印嵌入固定种子（不依赖用户密码）。
var watermarkSeed = []byte("StegGo::watermark::v2::offline")

// ErrNoWatermark 未找到水印。
var ErrNoWatermark = errors.New("未找到水印 (图像中未嵌入 StegGo 水印)")

// EmbedWatermark 将水印标记嵌入图像（LSB depth=1，固定种子）。
// 布局：[WMV2:][mark 长度 2B BE][mark]。水印无加密、公开可提取，适用于版权归属声明。
func (s *Service) EmbedWatermark(imgPath, outPath, mark string) (*Result, error) {
	start := time.Now()
	if mark == "" {
		return nil, errors.New("水印内容不能为空")
	}
	if len(mark) > 65535 {
		return nil, errors.New("水印内容过长")
	}
	payload := make([]byte, 0, len(watermarkPrefix)+2+len(mark))
	payload = append(payload, []byte(watermarkPrefix)...)
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(mark)))
	payload = append(payload, lb[:]...)
	payload = append(payload, []byte(mark)...)
	c := carrier.Get(carrier.KindImage)
	if c == nil {
		return nil, carrier.ErrUnsupportedFormat
	}
	copt := carrier.Options{Algorithm: "lsb", BitDepth: 1, Seed: watermarkSeed}
	if err := c.Embed(imgPath, outPath, payload, copt); err != nil {
		return nil, wrapErr("水印嵌入", err)
	}
	res := &Result{
		Name:    outPath,
		Size:    int64(len(mark)),
		OutPath: outPath,
		Elapsed: time.Since(start),
	}
	s.audit("watermark", imgPath, outPath, res, nil)
	return res, nil
}

// ExtractWatermark 从图像提取水印标记。
// 自动扫描 LSB 深度 1-4 与常见算法组合。
func (s *Service) ExtractWatermark(imgPath string) (string, error) {
	c := carrier.Get(carrier.KindImage)
	if c == nil {
		return "", carrier.ErrUnsupportedFormat
	}
	depths := []int{1, 2, 3, 4}
	for _, d := range depths {
		copt := carrier.Options{Algorithm: "lsb", BitDepth: d, Seed: watermarkSeed}
		stream, err := c.Extract(imgPath, copt)
		if err != nil {
			continue
		}
		if idx := bytes.Index(stream, []byte(watermarkPrefix)); idx >= 0 {
			body := stream[idx+len(watermarkPrefix):]
			if len(body) < 2 {
				continue
			}
			n := int(binary.BigEndian.Uint16(body[:2]))
			if n <= 0 || n > len(body)-2 {
				continue
			}
			return string(body[2 : 2+n]), nil
		}
	}
	return "", ErrNoWatermark
}
