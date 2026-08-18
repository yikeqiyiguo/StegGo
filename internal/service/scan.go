package service

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"steggo/internal/algorithm"
	"steggo/internal/carrier"
	"steggo/internal/common"
	"steggo/internal/crypto"
)

// combo 一次算法参数尝试组合。
type combo struct {
	algorithm string
	bitDepth  int
	quality   int
	levels    int
	costStyle string
}

// knownScanAlgo 已内建扫描参数集的算法。
func knownScanAlgo(name string) bool {
	switch name {
	case "lsb", "dct", "dwt", "hugo", "wow", "uniward":
		return true
	}
	return false
}

// adaptiveCostStyle 返回自适应算法的成本函数名。
// HUGO 默认使用 HILL 成本，WOW/UNIWARD 使用同名成本函数。
func adaptiveCostStyle(algo string) string {
	switch algo {
	case "wow":
		return "wow"
	case "uniward":
		return "uniward"
	default: // hugo 及其它
		return "hill"
	}
}

// buildScanCombos 从算法注册表动态构建提取扫描矩阵。
//
// 架构设计：扫描参数集由算法注册表驱动，避免"新算法注册后忘记加入
// 扫描矩阵"导致提取失败。对每种算法生成其完整常用参数空间：
//   - LSB：深度 2/1/3/4（2 为 V1.0 与默认深度，优先级靠前）
//   - DCT：质量 8/4/16/24/32（覆盖 CLI 常用自定义质量）
//   - DWT：级数 2/1/3
//   - 自适应：hill/wow/uniward 三种成本函数
//
// 注册表中未内建的第三方算法，退化为默认参数尝试一次。
func buildScanCombos() []combo {
	var out []combo
	if algorithm.Get("lsb") != nil {
		for _, d := range []int{2, 1, 3, 4} {
			out = append(out, combo{algorithm: "lsb", bitDepth: d})
		}
	}
	if algorithm.Get("dct") != nil {
		// 覆盖文档推荐 4~16 全部整数，兼容 CLI 自定义质量嵌入后 GUI 盲提取。
		for q := 4; q <= 16; q++ {
			out = append(out, combo{algorithm: "dct", quality: q})
		}
	}
	if algorithm.Get("dwt") != nil {
		for _, l := range []int{2, 1, 3} {
			out = append(out, combo{algorithm: "dwt", levels: l})
		}
	}
	for _, name := range []string{"hugo", "wow", "uniward"} {
		if algorithm.Get(name) != nil {
			// 成本函数名必须与 adaptive.costFor 的合法值一致：
			// HUGO 默认 HILL 成本，WOW/UNIWARD 用同名成本函数。
			out = append(out, combo{algorithm: name, costStyle: adaptiveCostStyle(name)})
		}
	}
	// 第三方/未知注册算法：默认参数兜底，保证插件化算法也能被盲提取。
	for _, n := range algorithm.Names() {
		if !knownScanAlgo(n) {
			out = append(out, combo{algorithm: n})
		}
	}
	return out
}

// scanImageExtract 对图像载体扫描算法矩阵提取载荷。
// 找到 V3（或 V2）魔数即命中，返回位流还原后的字节流。
func scanImageExtract(path string, opt Options, seed []byte) ([]byte, string, int, error) {
	c := carrier.Get(carrier.KindImage)
	if c == nil {
		return nil, "", 0, carrier.ErrUnsupportedFormat
	}

	// 用户显式指定的算法优先尝试一次。
	combos := make([]combo, 0, len(scanCombos())+1)
	if opt.Algorithm != "" {
		combos = append(combos, combo{
			algorithm: opt.Algorithm,
			bitDepth:  opt.BitDepth,
			quality:   opt.Quality,
			levels:    opt.Levels,
			costStyle: opt.CostStyle,
		})
	}
	combos = append(combos, scanCombos()...)

	var lastErr error
	attempted := make([]string, 0, len(combos))
	for _, cm := range combos {
		copt := opt.carrierOptions(seed)
		copt.Algorithm = cm.algorithm
		if cm.bitDepth > 0 {
			copt.BitDepth = cm.bitDepth
		}
		if cm.quality > 0 {
			copt.Quality = cm.quality
		}
		if cm.levels > 0 {
			copt.Levels = cm.levels
		}
		if cm.costStyle != "" {
			copt.CostStyle = cm.costStyle
		}
		stream, err := c.Extract(path, copt)
		if err != nil {
			// 特征点锚定对"非锚定嵌入"的图像报定位失败（找不到同步区）。
			// 这类错误仅说明该组合未嵌入载荷，不覆盖更精确的 lastErr
			// （如密码错误/载荷解析失败），否则扫描矩阵末尾的 anchored
			// 会掩盖真实失败原因，导致密码错误误报为"锚点不足"。
			if !errors.Is(err, algorithm.ErrNoAnchors) {
				lastErr = err
			}
			continue
		}
		attempted = append(attempted, describeCombo(cm))

		// ECC 包装流：先 RS 纠错解包，再检查 V3/V2 魔数。
		payloadStream := stream
		if len(stream) >= eccHeaderLen && bytes.HasPrefix(stream, eccMagic) {
			decoded, _, _, ok, uerr := unwrapECC(stream)
			if uerr != nil {
				lastErr = uerr
				continue
			}
			if !ok {
				lastErr = carrier.ErrNoPayload
				continue
			}
			payloadStream = decoded
		}
		if len(payloadStream) >= len(common.MagicV3) &&
			bytes.HasPrefix(payloadStream, []byte(common.MagicV3)) {
			// 关键：仅魔数命中不足以保证参数正确。例如 DCT 相邻 Quality 的量化
			// 网格奇偶一致，会整段命中魔数但密文区不同。必须验证载荷可完整
			// 解析（TrimPayload + 解密）后才视为命中，否则继续尝试下一组合。
			if verr := validateExtractedPayload(payloadStream, opt); verr == nil {
				// 命中：ECC 载荷返回原始流（由 resolveAndWrite 解包以获得纠错统计），
				// 普通载荷返回原流。
				return stream, cm.algorithm, cm.bitDepth, nil
			} else {
				lastErr = verr
				continue
			}
		}
		if len(payloadStream) >= len(common.MagicV2) &&
			bytes.HasPrefix(payloadStream, []byte(common.MagicV2)) {
			// V2 魔数命中的是 V1.0 旧载体，交给 service.Extract 的 V1 兼容回退处理
			return payloadStream, "lsb", cm.bitDepth, nil
		}
	}
	if lastErr == nil {
		lastErr = carrier.ErrNoPayload
	}
	return nil, "", 0, fmt.Errorf("%w（已尝试 %d 种算法参数组合: %s）",
		lastErr, len(attempted), strings.Join(attempted, "、"))
}

// scanCombos 惰性构建并缓存扫描矩阵（构建一次，复用）。
//
// 架构设计：这里不使用包级变量（var scanComboList = buildScanCombos()）初始化，
// 以避免对 Go 包初始化顺序的隐式依赖——若未来算法注册改为 init() 之外的惰性时机，
// 包级初始化会在算法注册完成前构建出"空扫描矩阵"，导致盲提取全部失败且无任何告警。
// 改用 sync.Once 在首次扫描时构建，保证无论算法注册何时完成，扫描矩阵总是
// 基于当前注册表的最新状态生成。
var scanOnce sync.Once

// scanCombos 返回惰性构建的扫描矩阵副本。
func scanCombos() []combo {
	scanOnce.Do(func() {
		scanComboList = buildScanCombos()
	})
	return scanComboList
}

// scanComboList 缓存扫描矩阵，由 scanCombos 惰性填充。
var scanComboList []combo

// validateExtractedPayload 验证命中魔数的位流对应的载荷可完整解析：
// TrimPayload 定位边界后，普通/可否认两种解析任一成功即视为有效组合。
// 消除"魔数误命中但参数不正确"导致的提前返回。
func validateExtractedPayload(stream []byte, opt Options) error {
	payload, err := crypto.TrimPayload(stream)
	if err != nil {
		return err
	}
	parseOpt := &crypto.ParseOptions{Password: opt.Password, KeyFile: opt.KeyFile}
	if _, _, err := crypto.ParsePayload(payload, parseOpt); err == nil {
		return nil
	}
	if _, _, _, err := crypto.ParseDeniablePayload(payload, opt.Password, parseOpt); err == nil {
		return nil
	}
	return errors.New("载荷解析失败（密码错误或参数不匹配）")
}

// describeCombo 生成组合的可读描述（用于诊断信息）。
func describeCombo(cm combo) string {
	var parts []string
	if cm.bitDepth > 0 {
		parts = append(parts, fmt.Sprintf("depth=%d", cm.bitDepth))
	}
	if cm.quality > 0 {
		parts = append(parts, fmt.Sprintf("q=%d", cm.quality))
	}
	if cm.levels > 0 {
		parts = append(parts, fmt.Sprintf("levels=%d", cm.levels))
	}
	if cm.costStyle != "" {
		parts = append(parts, cm.costStyle)
	}
	if len(parts) == 0 {
		return cm.algorithm
	}
	return cm.algorithm + "(" + strings.Join(parts, ",") + ")"
}
