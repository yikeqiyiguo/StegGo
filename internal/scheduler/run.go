package scheduler

import (
	"fmt"
	"path/filepath"

	"steggo/internal/service"
)

// Runner 批量任务执行器。
type Runner struct {
	Svc *service.Service
}

// Run 依次执行任务清单，返回汇总。
// 任务执行完成后立即清理内存中的密码。
func (r *Runner) Run(tasks []Task) Summary {
	if r.Svc == nil {
		r.Svc = service.New()
	}
	sum := Summary{Total: len(tasks)}
	for _, t := range tasks {
		if err := r.runOne(t); err != nil {
			sum.Failed++
			sum.Errors = append(sum.Errors, fmt.Sprintf("第 %d 行 [%s] %s: %v", t.Line, t.Action, t.Carrier, err))
			continue
		}
		sum.Success++
	}
	return sum
}

func (r *Runner) runOne(t Task) error {
	password := []byte(t.Password)
	defer wipeBytes(password)
	if len(password) == 0 {
		return fmt.Errorf("任务缺少密码（清单中必须显式提供 password）")
	}

	switch t.Action {
	case "hide", "embed":
		opt := service.Options{
			CarrierPath: t.Carrier,
			SecretPath:  t.Secret,
			OutputPath:  t.Output,
			Password:    password,
			Algorithm:   t.Algorithm,
			BitDepth:    t.BitDepth,
			UseSM4:      t.UseSM4,
		}
		if opt.BitDepth == 0 {
			opt.BitDepth = 1
		}
		if opt.OutputPath == "" {
			opt.OutputPath = t.Carrier + ".steg"
		}
		if opt.SecretPath == "" {
			return fmt.Errorf("hide 任务缺少 secret")
		}
		_, err := r.Svc.Embed(opt)
		return err

	case "extract", "reveal":
		opt := service.Options{
			CarrierPath: t.Carrier,
			OutputPath:  t.Output,
			Password:    password,
			Algorithm:   t.Algorithm,
		}
		if opt.OutputPath == "" {
			opt.OutputPath = filepath.Join(filepath.Dir(t.Carrier), "extract_out")
		}
		_, err := r.Svc.Extract(opt)
		return err

	case "sg-create", "sg-encrypt":
		if t.Output == "" {
			t.Output = t.Carrier + ".sg"
		}
		_, err := r.Svc.ContainerEncrypt(t.Carrier, t.Output, password, t.UseSM4)
		return err

	case "sg-open", "sg-decrypt":
		_, err := r.Svc.ContainerDecrypt(t.Carrier, t.Output, password)
		return err

	default:
		return fmt.Errorf("不支持的动作 %q（可选 hide/extract/sg-create/sg-open）", t.Action)
	}
}

// wipeBytes 安全清理密码字节。
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
