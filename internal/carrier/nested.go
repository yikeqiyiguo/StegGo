package carrier

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Layer 套娃单层定义。
type Layer struct {
	// CarrierPath 该层载体文件路径。
	CarrierPath string
	// OutPath 该层输出文件路径；为空时自动生成临时路径。
	OutPath string
	// Opt 该层载体参数（算法/种子/尾部标记等）。
	Opt Options
}

// NestedEmbed 套娃递归嵌套：
//
//	payload → 嵌入 carriers[0] → 结果字节作为载荷嵌入 carriers[1] → ... → 最终输出。
//
// 返回各层输出路径（outs[0] 为最内层，outs[len-1] 为最外层）。
func NestedEmbed(layers []Layer, payload []byte) ([]string, error) {
	if len(layers) == 0 {
		return nil, errors.New("套娃至少需要一层载体")
	}
	outs := make([]string, len(layers))
	current := payload
	for i, l := range layers {
		c, err := ForPath(l.CarrierPath)
		if err != nil {
			return nil, fmt.Errorf("第 %d 层载体无效: %w", i+1, err)
		}
		outPath := l.OutPath
		if outPath == "" {
			outPath = filepath.Join(os.TempDir(),
				fmt.Sprintf("steggo_nested_%d_%s", i, filepath.Base(l.CarrierPath)))
		}
		if err := c.Embed(l.CarrierPath, outPath, current, l.Opt); err != nil {
			return nil, fmt.Errorf("第 %d 层嵌入失败: %w", i+1, err)
		}
		outs[i] = outPath
		// 下一层的载荷 = 本层输出文件的完整字节。
		if i < len(layers)-1 {
			b, err := os.ReadFile(outPath)
			if err != nil {
				return nil, fmt.Errorf("读取第 %d 层输出: %w", i+1, err)
			}
			current = b
		}
	}
	return outs, nil
}

// NestedExtract 套娃递归提取：
//
//	从最外层载体提取 → 得到次外层文件的完整字节 → 写入临时文件 → 再提取 → ... → 最终载荷。
//
// depth 为嵌套层数（必须 ≥ 1）。opts 按 最外层→最内层 顺序提供参数（与
// NestedEmbed 的 layers 顺序相反），长度不足时复用最后一个。
// 中间层临时文件自动清理。
func NestedExtract(outerPath string, depth int, opts ...Options) ([]byte, error) {
	if depth < 1 {
		return nil, errors.New("套娃层数必须 ≥ 1")
	}
	optFor := func(i int) Options {
		if i < len(opts) {
			return opts[i]
		}
		if len(opts) > 0 {
			return opts[len(opts)-1]
		}
		return Options{}
	}

	cur := outerPath
	for i := 0; i < depth; i++ {
		c, err := ForPath(cur)
		if err != nil {
			return nil, fmt.Errorf("第 %d 层载体无效: %w", i+1, err)
		}
		data, err := c.Extract(cur, optFor(i))
		if err != nil {
			return nil, fmt.Errorf("第 %d 层提取失败: %w", i+1, err)
		}
		if i == depth-1 {
			return data, nil
		}
		// 中间层：按魔数选择扩展名写入临时文件。
		ext := nestedTempExt(data)
		tmp := filepath.Join(os.TempDir(),
			fmt.Sprintf("steggo_nested_tmp_%d_%d%s", os.Getpid(), i, ext))
		if err := os.WriteFile(tmp, data, 0600); err != nil {
			return nil, err
		}
		defer os.Remove(tmp)
		cur = tmp
	}
	return nil, errors.New("套娃提取流程异常")
}

// nestedTempExt 依据字节魔数为中间层选择临时文件扩展名。
func nestedTempExt(data []byte) string {
	kind, err := DetectKindBytes(data)
	if err != nil {
		return ".bin"
	}
	switch kind {
	case KindImage:
		return ".png"
	case KindAudio:
		return ".wav"
	case KindPDF:
		return ".pdf"
	case KindText:
		return ".txt"
	case KindVideo:
		return ".mp4"
	default:
		return ".bin"
	}
}
