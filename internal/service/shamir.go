package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"steggo/internal/carrier"
	"steggo/internal/common"
	"steggo/internal/crypto"
	v1steg "steggo/pkg/steg"
)

// ShamirOptions 分权（Shamir 密钥分享）配置。
type ShamirOptions struct {
	Options
	// Total 总份额数（分片载体数）。
	Total int
	// Threshold 恢复阈值（最少需要多少份）。
	Threshold int
	// CarrierPaths 分权嵌入时使用的载体文件列表（长度 = Total）。
	CarrierPaths []string
	// ShareDir 分片输出目录（嵌入）/ 分片载体所在目录（提取）。
	ShareDir string
}

// shamirSeed Shamir 分片嵌入/提取使用固定种子（分片无 V3 头，不依赖密码游走）。
var shamirSeed = []byte("StegGo::shamir::share::v2")

// SplitToCarriers 将秘密数据加密后按 Shamir 分片，依次嵌入 Total 个载体。
// 输出分片载体文件：ShareDir/share_01.png ... share_NN.png。
func (s *Service) SplitToCarriers(opt ShamirOptions) ([]*Result, error) {
	start := time.Now()
	if len(opt.Password) == 0 && len(opt.KeyFile) == 0 {
		return nil, ErrNoCredential
	}
	if opt.Total < 2 || opt.Threshold < 2 || opt.Threshold > opt.Total {
		return nil, errors.New("分权参数非法：2 ≤ Threshold ≤ Total")
	}
	if len(opt.CarrierPaths) != opt.Total {
		return nil, fmt.Errorf("载体数量 %d 与总份额 %d 不一致", len(opt.CarrierPaths), opt.Total)
	}
	if opt.ShareDir == "" {
		return nil, errors.New("分片输出目录不能为空")
	}
	if err := common.EnsureDir(opt.ShareDir); err != nil {
		return nil, err
	}

	// 1. 构建加密载荷（分片载体不关心算法层，统一用 lsb 嵌入）
	secret, name, isDir, err := s.readSecret(opt.Options)
	if err != nil {
		return nil, wrapErr("读取秘密数据", err)
	}
	defer common.Wipe(secret)
	buildOpt := &crypto.BuildOptions{
		Name: name, Algorithm: "lsb", BitDepth: 1, Compress: opt.Compress,
		Password: opt.Password, KeyFile: opt.KeyFile,
		UseKeyFile: opt.UseKeyFile, UseMachine: opt.UseMachine,
		IsDir: isDir,
	}
	payload, meta, err := crypto.BuildPayload(secret, buildOpt)
	if err != nil {
		return nil, wrapErr("构建加密载荷", err)
	}
	defer common.Wipe(payload)

	// 2. Shamir 分片
	shares, err := v1steg.SplitSecret(payload, opt.Total, opt.Threshold)
	if err != nil {
		return nil, wrapErr("Shamir 分片", err)
	}

	// 3. 分片嵌入载体：[分片长度 8B BE][分片数据]
	var results []*Result
	for i, share := range shares {
		blob := make([]byte, 8+len(share))
		binary.BigEndian.PutUint64(blob[:8], uint64(len(share)))
		copy(blob[8:], share)
		defer common.Wipe(blob)

		outPath := filepath.Join(opt.ShareDir, fmt.Sprintf("share_%02d%s", i+1, filepath.Ext(opt.CarrierPaths[i])))
		c, err := carrier.ForPath(opt.CarrierPaths[i])
		if err != nil {
			return results, wrapErr("载体识别", err)
		}
		copt := carrier.Options{Algorithm: "lsb", BitDepth: 1, Seed: shamirSeed}
		if err := c.Embed(opt.CarrierPaths[i], outPath, blob, copt); err != nil {
			return results, wrapErr(fmt.Sprintf("分片 %d 嵌入", i+1), err)
		}
		results = append(results, &Result{
			Name: filepath.Base(outPath), Size: meta.Size, OutPath: outPath,
			Elapsed: time.Since(start),
		})
	}
	return results, nil
}

// RecoverFromCarriers 从分片载体中恢复秘密：提取分片 → Shamir 恢复 → 解密 → 写出。
// 从 ShareDir 下自动收集 share_*.文件（或显式传入 CarrierPaths）。
func (s *Service) RecoverFromCarriers(opt ShamirOptions) (*Result, error) {
	start := time.Now()
	if len(opt.Password) == 0 && len(opt.KeyFile) == 0 {
		return nil, ErrNoCredential
	}
	if opt.Threshold < 2 {
		return nil, errors.New("恢复阈值必须 ≥ 2")
	}
	if opt.ShareDir == "" {
		return nil, errors.New("分片目录不能为空")
	}
	if err := common.EnsureDir(opt.ShareDir); err != nil {
		return nil, err
	}

	// 1. 收集分片载体（显式列表优先，否则扫描目录中全部受支持文件）
	var shareFiles []string
	if len(opt.CarrierPaths) > 0 {
		shareFiles = opt.CarrierPaths
	} else {
		exts := []string{".png", ".bmp", ".tif", ".tiff", ".wav", ".flac", ".pdf", ".txt", ".md", ".mp4", ".mkv"}
		if err := common.WalkFilesByExt(opt.ShareDir, exts, &shareFiles); err != nil {
			return nil, err
		}
	}
	if len(shareFiles) < opt.Threshold {
		return nil, fmt.Errorf("可用分片 %d < 阈值 %d", len(shareFiles), opt.Threshold)
	}

	// 2. 提取分片
	var shares [][]byte
	for _, f := range shareFiles {
		c, err := carrier.ForPath(f)
		if err != nil {
			continue
		}
		copt := carrier.Options{Algorithm: "lsb", BitDepth: 1, Seed: shamirSeed}
		stream, err := c.Extract(f, copt)
		if err != nil {
			continue
		}
		if len(stream) < 8 {
			continue
		}
		n := int(binary.BigEndian.Uint64(stream[:8]))
		if n <= 0 || n > len(stream)-8 {
			continue
		}
		shares = append(shares, stream[8:8+n])
	}
	if len(shares) < opt.Threshold {
		return nil, fmt.Errorf("成功提取分片 %d < 阈值 %d", len(shares), opt.Threshold)
	}

	// 3. Shamir 恢复 + 解密
	payload, err := v1steg.RecoverSecret(shares, opt.Threshold)
	if err != nil {
		return nil, wrapErr("Shamir 恢复", err)
	}
	defer common.Wipe(payload)

	plain, meta, err := crypto.ParsePayload(payload, &crypto.ParseOptions{Password: opt.Password, KeyFile: opt.KeyFile})
	if err != nil {
		return nil, wrapErr("载荷解密", err)
	}
	defer common.Wipe(plain)

	outDir := opt.OutputPath
	if outDir == "" {
		outDir = opt.ShareDir
	}
	if err := common.EnsureDir(outDir); err != nil {
		return nil, wrapErr("创建输出目录", err)
	}
	if err := writeExtracted(plain, meta, outDir); err != nil {
		return nil, wrapErr("写出数据", err)
	}
	return &Result{
		Name: meta.Name, Algorithm: meta.Algorithm, BitDepth: meta.BitDepth,
		IsDir: meta.IsDir, Size: meta.Size, OutPath: outDir,
		Elapsed: time.Since(start),
	}, nil
}
