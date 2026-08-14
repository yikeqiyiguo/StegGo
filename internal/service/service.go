// Package service 是 StegGo V2.0 业务服务层：
//
//	统一嵌入/提取（加密 + 载体 + 算法编排）、批量处理、
//	Shamir 分权多载体分发、数字水印、嵌套套娃、审计报告。
//
// 依赖 internal/crypto（载荷构建）与 internal/carrier（载体层），
// 并提供与 V1.0（pkg/steg）完全兼容的提取回退路径。
package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"steggo/internal/carrier"
	"steggo/internal/common"
	"steggo/internal/crypto"
	v1crypto "steggo/pkg/crypto"
	v1steg "steggo/pkg/steg"
)

// Service 聚合底层能力的高层业务服务。
type Service struct {
	// Audit 审计日志器（nil 表示不记录）。
	Audit *common.AuditLogger
}

// New 创建服务实例。
func New() *Service { return &Service{} }

// Options 一次嵌入/提取的统一配置。
type Options struct {
	// 输入输出
	CarrierPath string // 嵌入：载体文件；提取：含隐写数据的文件
	SecretPath  string // 嵌入：秘密文件或目录
	OutputPath  string // 嵌入：输出载体路径；提取：输出目录（默认当前目录）

	// 算法参数
	Algorithm   string  // lsb|dct|dwt|hugo|wow|uniward（默认 lsb）
	BitDepth    int     // LSB 每通道位数 1-4
	ChannelMask int     // 通道掩码 bit0=R bit1=G bit2=B
	BlockSize   int     // DCT/DWT 块大小（默认 8）
	Quality     int     // DCT 量化步长
	Levels      int     // DWT 级数
	CostStyle   string  // 自适应成本函数
	Gamma       float64 // 自适应成本指数

	// 加密因子
	Password   []byte // 密码（调用方负责清理）
	KeyFile    []byte // KeyFile 内容
	UseKeyFile bool
	UseMachine bool

	// 数据选项
	Name     string // 嵌入时覆盖文件名（默认取 SecretPath 文件名）
	Compress bool   // 是否启用 ZIP 压缩（默认启用小文件阈值自动判断）
	IsDir    bool   // SecretPath 为目录时自动识别

	// 可否认胁迫隐写
	FakeFile     string // 诱饵文件路径
	FakePassword []byte // 诱饵密码
}

// Result 一次操作的结果摘要。
type Result struct {
	Name        string
	Algorithm   string
	BitDepth    int
	IsDir       bool
	Deniable    bool
	Region      string // 可否认提取命中区：real|fake
	Size        int64  // 明文原始大小
	OutPath     string // 输出载体/输出目录
	CarrierSize int64  // 载体文件大小
	V1Compat    bool   // 是否通过 V1.0 兼容路径完成
	Elapsed     time.Duration
}

// serviceErr 带阶段的业务错误。
type serviceErr struct {
	stage string
	err   error
}

func (e *serviceErr) Error() string { return e.stage + ": " + e.err.Error() }
func (e *serviceErr) Unwrap() error { return e.err }

func wrapErr(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &serviceErr{stage: stage, err: err}
}

// ErrNoCredential 缺少加密因子。
var ErrNoCredential = errors.New("至少需要密码或密钥文件之一")

// deniableSeed 可否认载荷的固定定位种子（与密码无关）。
// 可否认隐写的核心目标是"无法证明哪份密文是真实的"，而非隐藏载荷存在性，
// 因此定位种子使用固定常量，保证真假密码都能定位双密文结构。
var deniableSeed = []byte("StegGo::deniable::payload::v2")

// validate 校验并填充默认值。
func (o *Options) validate() error {
	if o.CarrierPath == "" {
		return errors.New("载体文件路径不能为空")
	}
	if len(o.Password) == 0 && len(o.KeyFile) == 0 && !o.UseMachine {
		return ErrNoCredential
	}
	if o.Algorithm == "" {
		o.Algorithm = "lsb"
	}
	return nil
}

// deriveSeed 由三因子组合派生隐写坐标随机种子。
// 仅密码因子时与 V1.0（SeedFromPassword）完全一致，保证 V1.0 载体兼容。
func deriveSeed(secret []byte) []byte {
	return v1crypto.DeriveKey(secret, []byte(v1crypto.FixedSalt), v1crypto.DefaultIterations)
}

// carrierOptions 构造载体层参数。
func (o *Options) carrierOptions(seed []byte) carrier.Options {
	return carrier.Options{
		Algorithm:   o.Algorithm,
		BitDepth:    o.BitDepth,
		ChannelMask: o.ChannelMask,
		BlockSize:   o.BlockSize,
		Quality:     o.Quality,
		Levels:      o.Levels,
		CostStyle:   o.CostStyle,
		Gamma:       o.Gamma,
		Seed:        seed,
	}
}

// ---------------------------------------------------------------------------
// 嵌入
// ---------------------------------------------------------------------------

// Embed 将秘密数据加密后嵌入载体，输出到 OutputPath。
func (s *Service) Embed(opt Options) (*Result, error) {
	start := time.Now()
	if err := opt.validate(); err != nil {
		return nil, err
	}
	if opt.OutputPath == "" {
		return nil, errors.New("输出载体路径不能为空")
	}

	// 1. 读取秘密数据（文件或目录）
	secret, name, isDir, err := s.readSecret(opt)
	if err != nil {
		return nil, wrapErr("读取秘密数据", err)
	}
	defer common.Wipe(secret)

	// 2. 构建加密载荷（普通或可否认）
	// 头部位深仅作元数据（提取由扫描矩阵决定），未指定时写默认值 1（ParseV3Header 校验范围 1-4）。
	headBitDepth := opt.BitDepth
	if headBitDepth == 0 {
		headBitDepth = 1
	}
	buildOpt := &crypto.BuildOptions{
		Name:       name,
		Algorithm:  opt.Algorithm,
		BitDepth:   headBitDepth,
		Compress:   opt.Compress,
		Password:   opt.Password,
		KeyFile:    opt.KeyFile,
		UseKeyFile: opt.UseKeyFile,
		UseMachine: opt.UseMachine,
		IsDir:      isDir,
	}
	var (
		payload []byte
		meta    *crypto.Meta
	)
	if opt.FakeFile != "" && len(opt.FakePassword) > 0 {
		fake, ferr := os.ReadFile(opt.FakeFile)
		if ferr != nil {
			return nil, wrapErr("读取诱饵文件", ferr)
		}
		defer common.Wipe(fake)
		payload, meta, err = crypto.BuildDeniablePayload(
			&crypto.DeniablePayload{Real: secret, Fake: fake, Name: name},
			opt.Password, opt.FakePassword, buildOpt)
	} else {
		payload, meta, err = crypto.BuildPayload(secret, buildOpt)
	}
	if err != nil {
		return nil, wrapErr("构建加密载荷", err)
	}
	defer common.Wipe(payload)

	// 3. 派生坐标种子并嵌入载体。
	//    普通载荷与可否认载荷统一使用固定定位种子（见 deniableSeed 说明）。
	//    密码只参与载荷加解密，不再参与坐标游走定位。这样提取时：
	//      - 只要载体确已嵌入，任何算法都能定位载荷（算法行为一致）；
	//      - 密码错误时明确报"密码错误"，而不是令人困惑的"未找到载荷"；
	//      - 同一载体/算法/位深下，嵌入位置固定，重复嵌入可覆盖旧数据。
	seed := append([]byte(nil), deniableSeed...)
	defer common.Wipe(seed)

	c, err := carrier.ForPath(opt.CarrierPath)
	if err != nil {
		return nil, wrapErr("载体识别", err)
	}
	if err := c.Embed(opt.CarrierPath, opt.OutputPath, payload, opt.carrierOptions(seed)); err != nil {
		return nil, wrapErr("载体嵌入", err)
	}

	res := &Result{
		Name:        meta.Name,
		Algorithm:   meta.Algorithm,
		BitDepth:    meta.BitDepth,
		IsDir:       meta.IsDir,
		Deniable:    meta.Deniable,
		Size:        meta.Size,
		OutPath:     opt.OutputPath,
		CarrierSize: fileSize(opt.OutputPath),
		Elapsed:     time.Since(start),
	}
	s.audit("embed", opt.CarrierPath, opt.OutputPath, res, nil)
	return res, nil
}

// deriveSeedFromOptions 由三因子派生种子。
func (s *Service) deriveSeedFromOptions(opt Options) ([]byte, error) {
	tf := &crypto.ThreeFactor{Password: opt.Password, KeyFile: opt.KeyFile, UseMachine: opt.UseMachine}
	if err := tf.Validate(); err != nil {
		return nil, wrapErr("加密因子", err)
	}
	secret := tf.Combine()
	defer common.Wipe(secret)
	return deriveSeed(secret), nil
}

// readSecret 读取秘密数据：文件直读；目录递归 ZIP 打包。
func (s *Service) readSecret(opt Options) (data []byte, name string, isDir bool, err error) {
	if opt.SecretPath == "" {
		return nil, "", false, errors.New("秘密文件路径不能为空")
	}
	name = common.SafeFileName(opt.SecretPath)
	if opt.Name != "" {
		name = common.SafeFileName(opt.Name)
	}
	if opt.IsDir || common.IsDir(opt.SecretPath) {
		data, err = v1crypto.ZipDir(opt.SecretPath)
		if err != nil {
			return nil, "", false, err
		}
		return data, name, true, nil
	}
	data, err = os.ReadFile(opt.SecretPath)
	if err != nil {
		return nil, "", false, err
	}
	return data, name, false, nil
}

// ---------------------------------------------------------------------------
// 提取
// ---------------------------------------------------------------------------

// Extract 从载体提取并解密秘密数据，写出到 OutputPath 目录。
// 优先走 V2.0 流程（自动扫描算法参数）；失败后回退 V1.0 兼容路径。
func (s *Service) Extract(opt Options) (*Result, error) {
	start := time.Now()
	if err := opt.validate(); err != nil {
		return nil, err
	}
	outDir := opt.OutputPath
	if outDir == "" {
		outDir = "."
	}
	if err := common.EnsureDir(outDir); err != nil {
		return nil, wrapErr("创建输出目录", err)
	}

	// 1. 派生种子（图像载体扫描需要；尾部/文本载体无害）
	seed, err := s.deriveSeedFromOptions(opt)
	if err != nil {
		return nil, err
	}
	defer common.Wipe(seed)

	// 2. 提取载荷字节流并解析 V3 头。
	//    先试密码种子（兼容 V2 早期版本由密码种子定位的旧载体），
	//    再试固定种子（当前版本所有新载体统一使用固定定位种子，
	//    保证密码错误时也能定位载荷并明确报出"密码错误"）。
	for _, sd := range [][]byte{seed, deniableSeed} {
		stream, algo, bitDepth, derr := extractStream(opt.CarrierPath, opt, sd)
		if derr != nil {
			err = derr
			continue
		}
		res, perr := s.resolveAndWrite(stream, algo, bitDepth, opt, outDir, start, false)
		if perr == nil {
			return res, nil
		}
		err = perr
	}

	// 3. V1.0 兼容回退（含 V1 音频/文本/旧图像载体）
	v1res, v1err := s.extractV1Compat(opt, outDir)
	if v1err == nil {
		res := &Result{
			Name:      v1res.Name,
			Size:      v1res.RawSize,
			IsDir:     v1res.IsDir,
			OutPath:   outDir,
			V1Compat:  true,
			Elapsed:   time.Since(start),
		}
		s.audit("extract", opt.CarrierPath, outDir, res, nil)
		return res, nil
	}
	return nil, wrapErr("提取失败", extractErr(err, v1err))
}

// extractErr 聚合 V2 扫描与 V1 回退的错误，保留最多诊断信息。
func extractErr(v2err, v1err error) error {
	switch {
	case v2err != nil && v1err != nil:
		return fmt.Errorf("V2 流程失败: %v; V1.0 回退失败: %v", v2err, v1err)
	case v2err != nil:
		return v2err
	default:
		return v1err
	}
}

// resolveAndWrite 解析 V3 载荷并写出。
func (s *Service) resolveAndWrite(stream []byte, algo string, bitDepth int, opt Options, outDir string, start time.Time, v1 bool) (*Result, error) {
	payload, err := crypto.TrimPayload(stream)
	if err != nil {
		return nil, wrapErr("载荷定位", err)
	}
	parseOpt := &crypto.ParseOptions{Password: opt.Password, KeyFile: opt.KeyFile}

	plain, meta, perr := crypto.ParsePayload(payload, parseOpt)
	region := ""
	if perr != nil {
		// 可否认载荷：尝试双密文解析
		plain, region, meta, perr = crypto.ParseDeniablePayload(payload, opt.Password, parseOpt)
		if perr != nil {
			return nil, wrapErr("载荷解密", perr)
		}
	}
	defer common.Wipe(plain)

	werr := writeExtracted(plain, meta, outDir)
	if werr != nil {
		return nil, wrapErr("写出数据", werr)
	}
	res := &Result{
		Name:        meta.Name,
		Algorithm:   meta.Algorithm,
		BitDepth:    meta.BitDepth,
		IsDir:       meta.IsDir,
		Deniable:    meta.Deniable,
		Region:      region,
		Size:        meta.Size,
		OutPath:     outDir,
		CarrierSize: fileSize(opt.CarrierPath),
		V1Compat:    v1,
		Elapsed:     time.Since(start),
	}
	s.audit("extract", opt.CarrierPath, outDir, res, nil)
	return res, nil
}

// extractV1Compat 调用 V1.0 完整提取流程作为兼容回退。
func (s *Service) extractV1Compat(opt Options, outDir string) (*v1steg.Result, error) {
	if len(opt.Password) == 0 {
		return nil, errors.New("V1.0 载体需要密码")
	}
	return v1steg.AutoExtract(opt.CarrierPath, outDir, opt.Password)
}

// extractStream 提取载荷字节流。
// 图像载体需要扫描算法参数矩阵（见 scan.go）；其他载体直接提取。
func extractStream(path string, opt Options, seed []byte) ([]byte, string, int, error) {
	kind, err := carrier.DetectKind(path)
	if err != nil {
		return nil, "", 0, wrapErr("载体识别", err)
	}
	if kind == carrier.KindImage {
		return scanImageExtract(path, opt, seed)
	}
	c := carrier.Get(kind)
	if c == nil {
		return nil, "", 0, carrier.ErrUnsupportedFormat
	}
	stream, err := c.Extract(path, opt.carrierOptions(seed))
	if err != nil {
		return nil, "", 0, err
	}
	return stream, opt.Algorithm, opt.BitDepth, nil
}

// writeExtracted 写出解密数据：目录解 ZIP；单文件解 ZIP；普通文件直写。
func writeExtracted(plain []byte, meta *crypto.Meta, outDir string) error {
	if meta.IsDir {
		return v1crypto.UnzipBytes(plain, outDir)
	}
	if meta.IsZIP {
		name, data, err := v1crypto.UnzipSingleFile(plain)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outDir, common.SafeFileName(name)), data, 0o644)
	}
	if meta.Name == "" {
		return errors.New("载荷缺少文件名")
	}
	return os.WriteFile(filepath.Join(outDir, common.SafeFileName(meta.Name)), plain, 0o644)
}

// fileSize 返回文件大小（不存在返回 0）。
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// audit 写入审计日志（Audit 未启用时静默）。
func (s *Service) audit(action, in, out string, res *Result, err error) {
	if s.Audit == nil {
		return
	}
	detail := ""
	if res != nil {
		parts := []string{}
		if res.Name != "" {
			parts = append(parts, "name="+res.Name)
		}
		if res.Algorithm != "" {
			parts = append(parts, "algo="+res.Algorithm)
		}
		if res.Deniable {
			parts = append(parts, "deniable")
		}
		detail = strings.Join(parts, " ")
	}
	result := "ok"
	hash := ""
	if err != nil {
		result = "fail"
		detail = err.Error()
	} else if res != nil && res.CarrierSize > 0 {
		if h, herr := common.SHA256File(in); herr == nil && len(h) >= 8 {
			hash = fmt.Sprintf("%x", h[:8])
		}
	}
	_ = s.Audit.Log(common.AuditEntry{
		Action: action,
		Target: filepath.Base(in),
		Result: result,
		Detail: detail,
		Hash:   hash,
	})
}
