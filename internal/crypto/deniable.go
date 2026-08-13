package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"steggo/internal/common"
	v1crypto "steggo/pkg/crypto"
)

// DeniablePayload 描述一次可否认胁迫隐写。
//
// 单载体双独立密文：
//   - Fake：假密码可解出的诱饵文件（如无关的普通图片/文档）
//   - Real：主密码可解出的真实秘密文件
//
// 两个密文使用独立随机盐、nonce 与 SHA256 哈希，结构上完全对称，
// 无法从载体探测到存在第二份密文。
type DeniablePayload struct {
	Real []byte   // 真实文件内容
	Fake []byte   // 诱饵文件内容
	Name string   // 真实文件名（写入头部）
}

// BuildDeniablePayload 构建可否认双密文载荷。
// flags 传普通 BuildOptions 需要的算法/位深信息；真实与诱饵分别使用不同密码加密。
func BuildDeniablePayload(data *DeniablePayload, realPass, fakePass []byte, opt *BuildOptions) ([]byte, *Meta, error) {
	if data == nil || len(data.Real) == 0 || len(data.Fake) == 0 {
		return nil, nil, errors.New("可否认隐写需要真实与诱饵两份数据")
	}
	if len(realPass) == 0 || len(fakePass) == 0 {
		return nil, nil, errors.New("真实密码与诱饵密码都不能为空")
	}
	opt = cloneBuildOptions(opt)

	// 两个密文独立加密（同一算法层，同一 V1.0 密文布局）
	secretR := composeSecret(realPass, opt.KeyFile, opt.UseKeyFile, opt.UseMachine)
	secretF := composeSecret(fakePass, opt.KeyFile, opt.UseKeyFile, opt.UseMachine)
	defer common.Wipe(secretR)
	defer common.Wipe(secretF)

	ctR, err := v1crypto.Encrypt(data.Real, secretR)
	if err != nil {
		return nil, nil, err
	}
	ctF, err := v1crypto.Encrypt(data.Fake, secretF)
	if err != nil {
		return nil, nil, err
	}

	// 扩展头：[lenR 4B][lenF 4B][hashR 32B][hashF 32B]
	ext := make([]byte, deniableExtLen)
	binary.BigEndian.PutUint32(ext[0:4], uint32(len(ctR)))
	binary.BigEndian.PutUint32(ext[4:8], uint32(len(ctF)))
	hR := sha256.Sum256(ctR)
	hF := sha256.Sum256(ctF)
	copy(ext[8:40], hR[:])
	copy(ext[40:72], hF[:])

	flags := byte(0)
	if opt.Compress {
		flags |= flagZIP
	}
	flags |= flagDeniable
	if opt.UseKeyFile {
		flags |= flagKeyFile
	}
	if opt.UseMachine {
		flags |= flagMachine
	}
	algoID, _ := AlgoNameToID(opt.Algorithm)
	head := EncodeV3Header(&Header{
		Flags:     flags,
		Algorithm: algoID,
		BitDepth:  opt.BitDepth,
		Name:      data.Name,
		Salt:      ctR[:16],
		Nonce:     ctR[16:28],
		CipherLen: len(ctR) + len(ctF),
		CipherSum: sha256.Sum256(ctR),
	})

	out := make([]byte, 0, len(head)+len(ext)+len(ctR)+len(ctF))
	out = append(out, head...)
	out = append(out, ext...)
	out = append(out, ctR...)
	out = append(out, ctF...)

	meta := &Meta{
		Name:      data.Name,
		IsZIP:     opt.Compress,
		Algorithm: AlgoIDToName(algoID),
		BitDepth:  opt.BitDepth,
		Size:      int64(len(data.Real)),
		Deniable:  true,
	}
	return out, meta, nil
}

// ParseDeniablePayload 用给定密码尝试解开可否认载荷。
// 优先尝试第一区（诱饵），失败后尝试第二区（真实）。
// 返回 (明文, 所在区, 元信息)。
func ParseDeniablePayload(payload []byte, password []byte, opt *ParseOptions) ([]byte, string, *Meta, error) {
	head, headLen, err := ParseV3Header(payload)
	if err != nil {
		return nil, "", nil, err
	}
	if head.Flags&flagDeniable == 0 {
		return nil, "", nil, errors.New("非可否认载荷")
	}
	if len(payload) < headLen+deniableExtLen {
		return nil, "", nil, errors.New("可否认扩展头不完整")
	}
	ext := payload[headLen : headLen+deniableExtLen]
	lenR := int(binary.BigEndian.Uint32(ext[0:4]))
	lenF := int(binary.BigEndian.Uint32(ext[4:8]))
	body := payload[headLen+deniableExtLen:]
	if lenR+lenF != len(body) {
		return nil, "", nil, errors.New("可否认载荷长度不一致，载体可能已损坏")
	}
	ctR := body[:lenR]
	ctF := body[lenR:]
	if opt == nil {
		opt = &ParseOptions{}
	}

	// 先尝试第二区（真实），再尝试第一区（诱饵）——返回顺序保证主密码优先命中真实文件
	tryDecrypt := func(ct []byte, expectSum []byte, target string) ([]byte, error) {
		secret := composeSecret(password, opt.KeyFile, head.Flags&flagKeyFile != 0, head.Flags&flagMachine != 0)
		defer common.Wipe(secret)
		sum := sha256.Sum256(ct)
		if expectSum != nil && !v1crypto.ConstantTimeEqual(sum[:], expectSum) {
			return nil, errors.New("哈希不匹配")
		}
		return v1crypto.Decrypt(ct, secret)
	}

	// 主密码：先试真实区
	if pt, err := tryDecrypt(ctR, nil, "real"); err == nil {
		meta := &Meta{Name: head.Name, Algorithm: AlgoIDToName(head.Algorithm), BitDepth: head.BitDepth, Size: int64(len(pt)), Deniable: true}
		return pt, "real", meta, nil
	}
	// 假密码：再试诱饵区
	if pt, err := tryDecrypt(ctF, nil, "fake"); err == nil {
		meta := &Meta{Name: head.Name, Algorithm: AlgoIDToName(head.Algorithm), BitDepth: head.BitDepth, Size: int64(len(pt)), Deniable: true}
		return pt, "fake", meta, nil
	}
	return nil, "", nil, errors.New("解密失败：密码错误或数据已被篡改")
}

// cloneBuildOptions 复制 BuildOptions，避免修改调用方对象。
func cloneBuildOptions(opt *BuildOptions) *BuildOptions {
	if opt == nil {
		return &BuildOptions{}
	}
	c := *opt
	return &c
}
