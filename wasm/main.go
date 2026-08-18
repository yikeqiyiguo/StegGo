//go:build js && wasm

// StegGo WASM 入口：浏览器纯前端离线解析审计（只读，不写入任何文件）。
// 全部计算在浏览器本地完成，数据不出本机。
//
// 构建:
//
//	GOOS=js GOARCH=wasm go build -o dist/steggo.wasm ./wasm
//	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/
package main

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"syscall/js"

	"steggo/internal/algorithm"
	"steggo/internal/carrier"
)

const (
	// version WASM 审计版本
	version = "V2.2.0-wasm"
	// magicV3 / magicV2 载荷魔数
	magicV3 = "STEGGO3A"
	magicV2 = "STEGGO2A"
)

// analysisResult 审计结果 JSON 结构。
type analysisResult struct {
	Version   string  `json:"version"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	ChiSquare float64 `json:"chiSquare"` // 卡方 p 值（<0.05 疑似隐写）
	EmbedRate float64 `json:"embedRate"` // RS 分析估计嵌入率 [0,1]
	Verdict   string  `json:"verdict"`
	Magic     string  `json:"magic"`    // 扫描到的载荷魔数（空=未发现）
	MagicBit  int     `json:"magicBit"` // 魔数在 LSB 位流中的字节偏移
	Note      string  `json:"note"`
}

func main() {
	js.Global().Set("steggoVersion", js.FuncOf(func(this js.Value, args []js.Value) any {
		return version
	}))
	js.Global().Set("steggoAnalyze", js.FuncOf(analyzeFile))
	js.Global().Set("steggoScan", js.FuncOf(scanFile))
	// 阻塞保持 WASM 存活
	<-make(chan struct{})
}

// analyzeFile 纯前端审计：解码图像 → 卡方 + RS 分析。
func analyzeFile(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return jsonError("缺少输入文件")
	}
	data := bytesFromJS(args[0])
	img, err := carrier.DecodeImageBytes(data)
	if err != nil {
		return jsonError("图像解码失败: " + err.Error())
	}
	res := algorithm.Analyze(img, nil, algorithm.Options{})
	out := analysisResult{
		Version:   version,
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		ChiSquare: res.ChiSquare,
		EmbedRate: res.EmbedRate,
	}
	out.Verdict = "未发现明显隐写痕迹"
	if out.ChiSquare < 0.05 {
		out.Verdict = "卡方检验提示可能含有隐写数据（P=" + formatFloat(out.ChiSquare) + "）"
	}
	if out.EmbedRate > 0.05 {
		out.Verdict += "；RS 分析显示 LSB 平面存在异常（嵌入率≈" + formatFloat(out.EmbedRate*100) + "%）"
	}
	return marshal(out)
}

// scanFile 只读扫描：按 LSB 位流搜索 V2/V3 载荷魔数。
func scanFile(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return jsonError("缺少输入文件")
	}
	data := bytesFromJS(args[0])
	img, err := carrier.DecodeImageBytes(data)
	if err != nil {
		return jsonError("图像解码失败: " + err.Error())
	}
	res := algorithm.Analyze(img, nil, algorithm.Options{})
	out := analysisResult{
		Version:   version,
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		ChiSquare: res.ChiSquare,
		EmbedRate: res.EmbedRate,
	}

	magic, offset, ok := scanMagic(img)
	if ok {
		out.Magic = magic
		out.MagicBit = offset
		out.Note = "检测到 StegGo 载荷魔数：该图片疑似由 StegGo 隐写生成（只读审计，不做写入）"
		out.Verdict = "疑似含 StegGo 载荷"
	} else {
		out.Note = "未扫描到 StegGo 载荷魔数（基于 1 位 LSB 顺序扫描）"
		out.Verdict = "未发现 StegGo 载荷"
	}
	return marshal(out)
}

// scanMagic 按 1 位 LSB（先 MSB-first 再 LSB-first）扫描载荷魔数。
func scanMagic(img *image.NRGBA) (magic string, offset int, ok bool) {
	var bits []byte
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			bits = append(bits, c.R&1, c.G&1, c.B&1)
		}
	}
	// 位 → 字节，两种位序
	for _, msbFirst := range []bool{true, false} {
		stream := bitsToBytes(bits, msbFirst)
		for _, m := range []string{magicV3, magicV2} {
			idx := bytes.Index(stream, []byte(m))
			if idx >= 0 {
				return m, idx, true
			}
		}
	}
	return "", 0, false
}

// bitsToBytes 将位序列按两种位序之一组装为字节流。
func bitsToBytes(bits []byte, msbFirst bool) []byte {
	out := make([]byte, 0, len(bits)/8)
	for i := 0; i+7 < len(bits); i += 8 {
		var v byte
		for j := 0; j < 8; j++ {
			var bit byte
			if msbFirst {
				bit = bits[i+j] << (7 - j)
			} else {
				bit = bits[i+j] << j
			}
			v |= bit
		}
		out = append(out, v)
	}
	return out
}

// bytesFromJS 从 js.Value（Uint8Array / ArrayBuffer）提取 []byte。
func bytesFromJS(v js.Value) []byte {
	if v.Type() == js.TypeNull || v.Type() == js.TypeUndefined {
		return nil
	}
	if v.Type() == js.TypeObject && !v.IsUndefined() {
		if v.InstanceOf(js.Global().Get("Uint8Array")) {
			b := make([]byte, v.Get("length").Int())
			js.CopyBytesToGo(b, v)
			return b
		}
		if v.InstanceOf(js.Global().Get("ArrayBuffer")) {
			u8 := js.Global().Get("Uint8Array").New(v)
			b := make([]byte, u8.Get("length").Int())
			js.CopyBytesToGo(b, u8)
			return b
		}
	}
	return nil
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonError(msg string) string {
	return marshal(map[string]string{"error": msg})
}

func formatFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
