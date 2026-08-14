package service

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSeedMismatchRepro 防回归测试：嵌入密码与提取密码不一致时的行为。
//
// 背景：曾出现"GUI 中除 DCT 外其他算法提取全部失败（未找到载荷）"。
// 根因：坐标游走种子由密码派生，密码不一致时依赖种子的算法
// （lsb/dwt/hugo/wow/uniward）全部定位失败；而 DCT 顺序扫描、不依赖种子，
// 造成"DCT 成功、其他失败"的诡异差异。
//
// 修复：V2 嵌入统一使用固定定位种子（密码只参与加解密）。因此密码不一致时
// 所有算法都应定位到载荷并明确报"密码错误"，而不是"未找到载荷"。
func TestSeedMismatchRepro(t *testing.T) {
	carrierPath := filepath.Join("..", "..", "testdata", "carrier.png")
	secretPath := filepath.Join("..", "..", "testdata", "secret.txt")
	algos := []string{"lsb", "dct", "dwt", "hugo", "wow", "uniward"}
	dir := t.TempDir()

	for _, algo := range algos {
		out := filepath.Join(dir, "carrier_"+algo+".steg")
		if _, err := New().Embed(Options{
			CarrierPath: carrierPath,
			SecretPath:  secretPath,
			OutputPath:  out,
			Password:    []byte("embedpass"),
			Algorithm:   algo,
			BitDepth:    2,
		}); err != nil {
			t.Errorf("[%s] 嵌入失败: %v", algo, err)
			continue
		}

		// 提取时使用不同密码 → 密码不匹配（但定位种子固定，应能找到载荷）
		_, err := New().Extract(Options{
			CarrierPath: out,
			OutputPath:  filepath.Join(dir, "ex_"+algo),
			Password:    []byte("wrongpass"),
		})
		if err == nil {
			t.Errorf("[%s] 密码错误却提取成功（异常）", algo)
			continue
		}
		msg := err.Error()
		if strings.Contains(msg, "未找到 StegGo 载荷") {
			t.Errorf("[%s] 密码不一致报成了'未找到载荷'（异常，应为明确密码错误）: %s", algo, msg)
		} else if !strings.Contains(msg, "密码错误") {
			t.Errorf("[%s] 错误信息未提示密码错误: %s", algo, msg)
		} else {
			t.Logf("[%s] 密码不一致 → 明确报密码错误 ✓", algo)
		}

		// 正确密码应能成功提取
		if _, err := New().Extract(Options{
			CarrierPath: out,
			OutputPath:  filepath.Join(dir, "ok_"+algo),
			Password:    []byte("embedpass"),
		}); err != nil {
			t.Errorf("[%s] 正确密码提取失败: %v", algo, err)
		} else {
			t.Logf("[%s] 正确密码提取成功 ✓", algo)
		}
	}
}
