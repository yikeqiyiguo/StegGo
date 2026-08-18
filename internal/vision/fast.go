// Package vision 提供纯 Go 图像视觉原语（零外部依赖），
// 供特征点锚定隐写（internal/algorithm/anchored）定位几何锚点使用。
//
// 当前包含：
//   - FAST-9 角点检测（含非极大值抑制与响应排序）
//   - BT.601 灰度转换
//   - 90° 整数旋转（精确像素重排，无插值误差）
package vision

import (
	"image"
	"sort"
)

// KeyPoint 检测到的特征点。
type KeyPoint struct {
	X, Y     int
	Response int
}

// fastCircle 半径 3 圆周的 16 个像素偏移（FAST-9）。
// 按顺时针顺序排列，便于连续弧段扫描。
var fastCircle = [16][2]int{
	{0, -3}, {1, -3}, {2, -2}, {3, -1}, {3, 0}, {3, 1}, {2, 2}, {1, 3},
	{0, 3}, {-1, 3}, {-2, 2}, {-3, 1}, {-3, 0}, {-3, -1}, {-2, -2}, {-1, -3},
}

// FAST 检测 FAST-9 角点，返回按响应降序排列的结果（同响应按 y、x 升序稳定）。
// gray 为灰度平面（行主序）；threshold 为亮度差阈值（<=0 时默认 20）。
func FAST(gray []byte, w, h, threshold int) []KeyPoint {
	if threshold <= 0 {
		threshold = 20
	}
	if w < 10 || h < 10 {
		return nil
	}
	resp := make([]int, w*h)
	type cand struct{ x, y int }
	cands := make([]cand, 0, 512)
	for y := 3; y < h-3; y++ {
		row := y * w
		for x := 3; x < w-3; x++ {
			c := int(gray[row+x])
			// 快速预检：上下左右 4 个十字点中至少 3 个超出阈值。
			// 这是 FAST 论文的标准预筛，跳过绝大多数平坦像素。
			n := 0
			if absI(int(gray[row-3*w+x])-c) > threshold {
				n++
			}
			if absI(int(gray[row+3*w+x])-c) > threshold {
				n++
			}
			if absI(int(gray[row+x+3])-c) > threshold {
				n++
			}
			if absI(int(gray[row+x-3])-c) > threshold {
				n++
			}
			if n < 3 {
				continue
			}
			// 完整 16 点检查：至少 9 个连续像素全亮或全暗。
			var bright, dark [16]bool
			for i := 0; i < 16; i++ {
				v := int(gray[(y+fastCircle[i][1])*w+x+fastCircle[i][0]])
				if v > c+threshold {
					bright[i] = true
				}
				if v < c-threshold {
					dark[i] = true
				}
			}
			if longestRun(bright) < 9 && longestRun(dark) < 9 {
				continue
			}
			r := segmentResponse(gray, w, x, y, c, bright, dark)
			if r <= 0 {
				continue
			}
			resp[y*w+x] = r
			cands = append(cands, cand{x: x, y: y})
		}
	}
	// 非极大值抑制：3×3 邻域内保留响应最大者。
	out := make([]KeyPoint, 0, len(cands))
	for _, p := range cands {
		r := resp[p.y*w+p.x]
		best := true
		for dy := -1; dy <= 1 && best; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				if resp[(p.y+dy)*w+p.x+dx] > r {
					best = false
					break
				}
			}
		}
		if best {
			out = append(out, KeyPoint{X: p.x, Y: p.y, Response: r})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Response != out[j].Response {
			return out[i].Response > out[j].Response
		}
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// longestRun 计算环形标记序列中最长连续 true 弧段长度。
func longestRun(mark [16]bool) int {
	best, cur := 0, 0
	for i := 0; i < 32; i++ {
		if mark[i%16] {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	if best > 16 {
		best = 16
	}
	return best
}

// segmentResponse 角点响应：亮/暗连续弧段上与中心亮度差的绝对值之和的最大值。
func segmentResponse(gray []byte, w, x, y, c int, bright, dark [16]bool) int {
	best := 0
	scan := func(mark [16]bool) {
		for i := 0; i < 16; i++ {
			if !mark[i] {
				continue
			}
			sum := 0
			for j := 0; j < 16; j++ {
				idx := (i + j) % 16
				if !mark[idx] {
					break
				}
				v := int(gray[(y+fastCircle[idx][1])*w+x+fastCircle[idx][0]])
				sum += absI(v - c)
			}
			if sum > best {
				best = sum
			}
		}
	}
	scan(bright)
	scan(dark)
	return best
}

// ToGray 将任意 image.Image 转为 BT.601 灰度平面（行主序）。
func ToGray(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := make([]byte, w*h)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			gray[i] = YFromRGB(uint8(r>>8), uint8(g>>8), uint8(bl>>8))
			i++
		}
	}
	return gray
}

// YFromRGB 由 RGB 计算 BT.601 亮度。
func YFromRGB(r, g, b uint8) uint8 {
	return uint8((299*int(r) + 587*int(g) + 114*int(b)) / 1000)
}

// Rotate90 顺时针旋转 d 次 90°（d 取模 4）。整数像素重排，无插值误差。
// 返回新灰度平面与新尺寸（宽高互换）。
func Rotate90(gray []byte, w, h, d int) ([]byte, int, int) {
	cur, cw, ch := gray, w, h
	for k := 0; k < (d%4+4)%4; k++ {
		nw, nh := ch, cw
		out := make([]byte, nw*nh)
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				// 顺时针 90°：(x,y) → 新坐标 (ch-1-y, x)
				out[x*nw+ch-1-y] = cur[y*cw+x]
			}
		}
		cur, cw, ch = out, nw, nh
	}
	return cur, cw, ch
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
