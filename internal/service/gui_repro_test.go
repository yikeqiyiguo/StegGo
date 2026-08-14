package service

import (
	"path/filepath"
	"testing"
)

// TestGUIScenarioRepro 回归测试：精确复现 GUI 的嵌入/提取调用参数。
//
// 背景：曾出现"GUI 中除 DCT 外其他算法提取全部失败"的报告。历史上叠加了
// 三个问题：DWT 位翻转、uniwward 拼写错误、以及坐标种子依赖密码导致的
// "密码不一致时仅 DCT（顺序扫描不依赖种子）可提取"的诡异差异（后者已通过
// 统一固定定位种子修复，见 seed_repro_test.go）。为保证此类问题永不复发，
// 此测试把 GUI 的完整参数矩阵钉死在这里：
//  1. 嵌入：bits=2（GUI 默认），输出为 carrier+".steg"（GUI 默认命名）
//  2. 提取模式 A：自动扫描（GUI 默认）
//  3. 提取模式 B：显式选择算法（GUI 提取页下拉），BitDepth=0（未勾高级参数）
//  4. 提取模式 C：显式选择算法 + 高级参数 bits=2
func TestGUIScenarioRepro(t *testing.T) {
	carrierPath := filepath.Join("..", "..", "testdata", "carrier.png")
	secretPath := filepath.Join("..", "..", "testdata", "secret.txt")
	algos := []string{"lsb", "dct", "dwt", "hugo", "wow", "uniward"}
	dir := t.TempDir()

	for _, algo := range algos {
		// GUI 嵌入：bits=2，输出 carrier.png.steg
		out := filepath.Join(dir, "carrier_"+algo+".steg")
		_, err := New().Embed(Options{
			CarrierPath: carrierPath,
			SecretPath:  secretPath,
			OutputPath:  out,
			Password:    []byte("testpass"),
			Algorithm:   algo,
			BitDepth:    2,
		})
		if err != nil {
			t.Errorf("[%s] 嵌入失败: %v", algo, err)
			continue
		}

		// 模式 A：自动扫描
		if _, err := New().Extract(Options{
			CarrierPath: out,
			OutputPath:  filepath.Join(dir, "a_"+algo),
			Password:    []byte("testpass"),
		}); err != nil {
			t.Errorf("[%s] 模式A(自动扫描) 提取失败: %v", algo, err)
		} else {
			t.Logf("[%s] 模式A 通过", algo)
		}

		// 模式 B：显式选算法，未勾高级参数（BitDepth=0）
		if _, err := New().Extract(Options{
			CarrierPath: out,
			OutputPath:  filepath.Join(dir, "b_"+algo),
			Password:    []byte("testpass"),
			Algorithm:   algo,
		}); err != nil {
			t.Errorf("[%s] 模式B(显式算法,无高级参数) 提取失败: %v", algo, err)
		} else {
			t.Logf("[%s] 模式B 通过", algo)
		}

		// 模式 C：显式选算法 + 高级参数 bits=2
		if _, err := New().Extract(Options{
			CarrierPath: out,
			OutputPath:  filepath.Join(dir, "c_"+algo),
			Password:    []byte("testpass"),
			Algorithm:   algo,
			BitDepth:    2,
		}); err != nil {
			t.Errorf("[%s] 模式C(显式算法,高级bits=2) 提取失败: %v", algo, err)
		} else {
			t.Logf("[%s] 模式C 通过", algo)
		}
	}
}
