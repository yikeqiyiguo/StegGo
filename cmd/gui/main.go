// StegGo GUI - 桌面图形界面（Fyne）
//
// 注意：本目录是独立 Go Module（cmd/gui/go.mod）。
// Fyne 在 Windows/macOS/Linux 桌面端依赖 cgo（OpenGL/GLFW），
// 因此构建本 GUI 需要安装 C 编译器（Windows: MinGW-w64 / TDM-GCC）。
// 构建：cd cmd/gui && CGO_ENABLED=1 go build -o steggo-gui.exe .
//
// 功能统一：本界面通过 steggo/pkg/steg/sdk（V2.0 公开 SDK）驱动，
// 与 CLI/TUI 共用 internal/service 六算法嵌入、自动扫描提取、
// 水印、批量、Shamir 分权、容量/质量/自检审计等全部能力。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"steggo/pkg/steg/sdk"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(newGreenTheme())
	w := a.NewWindow("StegGo V2.0 - 抗检测隐写工具")
	w.Resize(fyne.NewSize(920, 700))
	w.CenterOnScreen()

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("嵌入", theme.UploadIcon(), newHideTab(w)),
		container.NewTabItemWithIcon("提取", theme.DownloadIcon(), newExtractTab(w)),
		container.NewTabItemWithIcon("水印", theme.DocumentIcon(), newWatermarkTab(w)),
		container.NewTabItemWithIcon("容量", theme.StorageIcon(), newCapacityTab(w)),
		container.NewTabItemWithIcon("质量", theme.MediaPhotoIcon(), newQualityTab(w)),
		container.NewTabItemWithIcon("自检审计", theme.VisibilityIcon(), newAuditTab(w)),
		container.NewTabItemWithIcon("批量", theme.FolderIcon(), newBatchTab(w)),
		container.NewTabItemWithIcon("关于", theme.InfoIcon(), newAboutTab()),
	)
	w.SetContent(container.NewBorder(appHeader(), nil, nil, nil, tabs))
	w.ShowAndRun()
}

// =============================================================
// 通用组件
// =============================================================

// browseBtn 返回一个文件/目录选择按钮，结果写入 entry。
func browseBtn(w fyne.Window, entry *widget.Entry, isDir bool) fyne.CanvasObject {
	return widget.NewButtonWithIcon("浏览", theme.FolderOpenIcon(), func() {
		if isDir {
			dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
				if err == nil && lu != nil {
					entry.SetText(lu.Path())
				}
			}, w).Show()
		} else {
			dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
				if err == nil && rc != nil {
					entry.SetText(rc.URI().Path())
					rc.Close()
				}
			}, w).Show()
		}
	})
}

// setText 在主线程安全地更新 UI 组件文本。
func setText(label *widget.Label, s string) {
	fyne.Do(func() { label.SetText(s) })
}

// runAsync 在后台 goroutine 执行 fn，并用状态区更新进度。
// 失败时同时弹出带滚动条的完整错误对话框，避免长错误被截断。
func runAsync(w fyne.Window, status *widget.Entry, fn func() error) {
	uiSetStatus(status, "执行中...", false)
	go func() {
		err := fn()
		if err != nil {
			uiSetStatus(status, err.Error(), true)
			uiErrorDialog(w, err)
			return
		}
		uiSetStatus(status, "完成", false)
	}()
}

func showInfo(w fyne.Window, title string, msg string) {
	uiSuccessDialog(w, title, msg)
}

// sectionTitle 返回页内节标题。
func sectionTitle(s string) fyne.CanvasObject {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// appHeader 返回应用顶部标题栏。
func appHeader() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("StegGo 隐写工具", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabelWithStyle("抗检测隐写 · 六算法 · AES-256-GCM · 自检审计 · 全平台", fyne.TextAlignLeading, fyne.TextStyle{})
	subtitle.Wrapping = fyne.TextWrapWord
	header := container.NewStack(
		canvas.NewRectangle(theme.Color(theme.ColorNameHeaderBackground)),
		container.NewPadded(container.NewVBox(title, subtitle)),
	)
	return container.NewVBox(header, widget.NewSeparator())
}

// parseBits 解析嵌入位数输入，返回 1-4。
func parseBits(s string) (int, error) {
	b := 2
	if s == "" {
		return b, nil
	}
	if _, err := fmt.Sscanf(s, "%d", &b); err != nil || b < 1 || b > 4 {
		return 0, fmt.Errorf("嵌入位数必须是 1-4 的整数")
	}
	return b, nil
}

// =============================================================
// 嵌入页
// =============================================================

func newHideTab(w fyne.Window) fyne.CanvasObject {
	carrier := widget.NewEntry()
	secret := widget.NewEntry()
	output := widget.NewEntry()
	password := widget.NewPasswordEntry()
	bits := widget.NewEntry()
	bits.SetText("2")

	algo := widget.NewSelect(sdk.Algorithms(), nil)
	algo.SetSelected("lsb")

	status := uiStatus("就绪")

	form := widget.NewForm(
		widget.NewFormItem("载体文件", container.NewBorder(nil, nil, nil, browseBtn(w, carrier, false), carrier)),
		widget.NewFormItem("秘密文件", container.NewBorder(nil, nil, nil, browseBtn(w, secret, false), secret)),
		widget.NewFormItem("输出文件", container.NewBorder(nil, nil, nil, browseBtn(w, output, false), output)),
		widget.NewFormItem("加密密码", password),
		widget.NewFormItem("隐写算法", algo),
		widget.NewFormItem("嵌入位数(1-4)", bits),
	)

	runBtn := uiIconButton("开始嵌入", theme.ConfirmIcon(), func() {
		if carrier.Text == "" || secret.Text == "" {
			dialog.ShowError(fmt.Errorf("请填写载体与秘密文件"), w)
			return
		}
		if len(password.Text) == 0 {
			dialog.ShowError(fmt.Errorf("密码不能为空"), w)
			return
		}
		out := output.Text
		if out == "" {
			out = carrier.Text + ".steg"
		}
		b, err := parseBits(bits.Text)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		opts := sdk.Options{
			CarrierPath: carrier.Text,
			SecretPath:  secret.Text,
			OutputPath:  out,
			Password:    []byte(password.Text),
			Algorithm:   algo.Selected,
			BitDepth:    b,
		}
		runAsync(w, status, func() error {
			res, err := sdk.Embed(opts)
			if err != nil {
				return err
			}
			// 嵌入后立即自动验证提取，确保输出文件可逆提取。
			// 这样嵌入侧的问题当场暴露，不会等到提取时才报"未找到载荷"。
			verifyDir := filepath.Join(filepath.Dir(out), ".steggo_verify")
			vopts := sdk.Options{
				CarrierPath: out,
				OutputPath:  verifyDir,
				Password:    []byte(password.Text),
			}
			if verr := os.RemoveAll(verifyDir); verr != nil {
				// 验证目录清理失败不阻塞，继续尝试
			}
			vres, verr := sdk.Extract(vopts)
			if verr != nil {
				showInfo(w, "警告", fmt.Sprintf("嵌入成功但自动验证失败!\n秘密: %s (%d B)\n算法: %s / %d 位\n输出: %s\n\n验证错误: %v\n\n请用此文件提取时使用相同密码: %s",
					res.Name, res.Size, res.Algorithm, res.BitDepth, out, verr, password.Text))
				return fmt.Errorf("嵌入成功但自动验证提取失败: %v（输出文件已生成: %s）", verr, out)
			}
			os.RemoveAll(verifyDir)
			showInfo(w, "完成", fmt.Sprintf("嵌入成功（已自动验证可提取）\n秘密: %s (%d B)\n算法: %s / %d 位\n输出: %s\n验证算法: %s",
				res.Name, res.Size, res.Algorithm, res.BitDepth, out, vres.Algorithm))
			return nil
		})
	})

	card := uiCard("参数设置", form)
	actionBar := container.NewBorder(nil, nil, nil, runBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("嵌入 - 将秘密文件藏入载体"),
		card,
		actionBar,
	)))
}

// =============================================================
// 提取页
// =============================================================

func newExtractTab(w fyne.Window) fyne.CanvasObject {
	carrier := widget.NewEntry()
	output := widget.NewEntry()
	password := widget.NewPasswordEntry()
	status := uiStatus("就绪")

	// 算法选择：默认"自动扫描"；选择具体算法时优先按该算法参数提取，
	// 失败后仍会回退到自动扫描全部组合，保证旧载体也能提取。
	algoOpts := append([]string{"自动扫描"}, sdk.Algorithms()...)
	algo := widget.NewSelect(algoOpts, nil)
	algo.SetSelected("自动扫描")

	// 高级参数（留空 = 默认值 / 自动扫描覆盖）
	adv := widget.NewCheck("使用高级参数（手动指定嵌入位数 / DCT 质量 / DWT 级数）", nil)
	bits := widget.NewEntry()
	bits.SetPlaceHolder("嵌入位数 1-4（如 2）")
	quality := widget.NewEntry()
	quality.SetPlaceHolder("DCT 质量 1-32（如 8）")
	levels := widget.NewEntry()
	levels.SetPlaceHolder("DWT 级数 1-3（如 2）")

	form := widget.NewForm(
		widget.NewFormItem("隐写载体", container.NewBorder(nil, nil, nil, browseBtn(w, carrier, false), carrier)),
		widget.NewFormItem("输出目录", container.NewBorder(nil, nil, nil, browseBtn(w, output, true), output)),
		widget.NewFormItem("解密密码", password),
		widget.NewFormItem("提取算法", algo),
		widget.NewFormItem("", adv),
		widget.NewFormItem("嵌入位数", bits),
		widget.NewFormItem("DCT 质量", quality),
		widget.NewFormItem("DWT 级数", levels),
	)
	runBtn := uiIconButton("开始提取", theme.SearchIcon(), func() {
		if carrier.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择载体文件"), w)
			return
		}
		if len(password.Text) == 0 {
			dialog.ShowError(fmt.Errorf("密码不能为空"), w)
			return
		}
		out := output.Text
		if out == "" {
			out = "./extracted"
		}
		opts := sdk.Options{
			CarrierPath: carrier.Text,
			OutputPath:  out,
			Password:    []byte(password.Text),
		}
		if algo.Selected != "" && algo.Selected != "自动扫描" {
			opts.Algorithm = algo.Selected
		}
		if adv.Checked {
			if v, err := parseBits(bits.Text); err == nil {
				opts.BitDepth = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(quality.Text)); err == nil && v >= 1 && v <= 32 {
				opts.Quality = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(levels.Text)); err == nil && v >= 1 && v <= 3 {
				opts.Levels = v
			}
		}
		runAsync(w, status, func() error {
			res, err := sdk.Extract(opts)
			if err != nil {
				return err
			}
			showInfo(w, "完成", fmt.Sprintf("提取成功\n秘密: %s (%d B)\n算法: %s / %d 位\n目录: %s",
				res.Name, res.Size, res.Algorithm, res.BitDepth, out))
			return nil
		})
	})
	card := uiCard("参数设置", form)
	actionBar := container.NewBorder(nil, nil, nil, runBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("提取 - 自动扫描算法并从载体还原秘密"),
		card,
		actionBar,
	)))
}

// =============================================================
// 水印页
// =============================================================

func newWatermarkTab(w fyne.Window) fyne.CanvasObject {
	img := widget.NewEntry()
	mark := widget.NewEntry()
	out := widget.NewEntry()
	password := widget.NewPasswordEntry()
	status := uiStatus("就绪")

	markForm := widget.NewForm(
		widget.NewFormItem("图像文件", container.NewBorder(nil, nil, nil, browseBtn(w, img, false), img)),
		widget.NewFormItem("水印内容", mark),
		widget.NewFormItem("输出文件", container.NewBorder(nil, nil, nil, browseBtn(w, out, false), out)),
	)
	embedBtn := uiIconButton("嵌入水印", theme.ConfirmIcon(), func() {
		if img.Text == "" || mark.Text == "" {
			dialog.ShowError(fmt.Errorf("请填写图像与水印内容"), w)
			return
		}
		outPath := out.Text
		if outPath == "" {
			outPath = img.Text + ".wm"
		}
		runAsync(w, status, func() error {
			res, err := sdk.EmbedWatermark(img.Text, outPath, mark.Text)
			if err != nil {
				return err
			}
			showInfo(w, "完成", fmt.Sprintf("水印嵌入成功\n输出: %s", res.OutPath))
			return nil
		})
	})

	extractForm := widget.NewForm(
		widget.NewFormItem("含水印图像", container.NewBorder(nil, nil, nil, browseBtn(w, img, false), img)),
		widget.NewFormItem("解密密码", password),
	)
	extractBtn := uiSecondaryButton("提取水印", theme.SearchIcon(), func() {
		if img.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择含水印的图像"), w)
			return
		}
		runAsync(w, status, func() error {
			m, err := sdk.ExtractWatermark(img.Text)
			if err != nil {
				return err
			}
			showInfo(w, "完成", fmt.Sprintf("提取到的水印:\n%s", m))
			return nil
		})
	})

	embedCard := uiCard("嵌入水印", markForm)
	extractCard := uiCard("提取水印", extractForm)
	actionBar := container.NewBorder(nil, nil, nil, extractBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("水印 - 版权归属声明"),
		embedCard,
		container.NewPadded(embedBtn),
		extractCard,
		actionBar,
	)))
}

// =============================================================
// 容量页
// =============================================================

func newCapacityTab(w fyne.Window) fyne.CanvasObject {
	img := widget.NewEntry()
	algo := widget.NewSelect(sdk.Algorithms(), nil)
	algo.SetSelected("lsb")
	status := uiStatus("就绪")
	result := widget.NewMultiLineEntry()
	result.Disable()
	result.SetPlaceHolder("结果将显示在这里")

	runBtn := uiIconButton("计算容量", theme.InfoIcon(), func() {
		if img.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择图片"), w)
			return
		}
		uiSetStatus(status, "计算中...", false)
		go func() {
			mat, err := sdk.CapacityMatrix(img.Text, algo.Selected)
			if err != nil {
				uiSetStatus(status, err.Error(), true)
				return
			}
			var s strings.Builder
			s.WriteString(fmt.Sprintf("算法: %s\n", algo.Selected))
			for _, r := range mat {
				s.WriteString(fmt.Sprintf("  %d 位: 最大 %d 字节\n", r.BitDepth, r.MaxBytes))
			}
			fyne.Do(func() {
				result.SetText(s.String())
				uiSetStatus(status, "计算完成", false)
			})
		}()
	})

	form := widget.NewForm(
		widget.NewFormItem("图片文件", container.NewBorder(nil, nil, nil, browseBtn(w, img, false), img)),
		widget.NewFormItem("算法", algo),
	)
	card := uiCard("参数设置", form)
	actionBar := container.NewBorder(nil, nil, nil, runBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("容量 - 估算图像可承载的秘密大小"),
		card,
		actionBar,
		uiCard("结果", result),
	)))
}

// =============================================================
// 质量页
// =============================================================

func newQualityTab(w fyne.Window) fyne.CanvasObject {
	orig := widget.NewEntry()
	steg := widget.NewEntry()
	status := uiStatus("就绪")
	result := widget.NewMultiLineEntry()
	result.Disable()
	result.SetPlaceHolder("结果将显示在这里")

	runBtn := uiIconButton("评估质量", theme.InfoIcon(), func() {
		if orig.Text == "" || steg.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择原图与隐写图"), w)
			return
		}
		uiSetStatus(status, "评估中...", false)
		go func() {
			rep, err := sdk.EvaluateQuality(orig.Text, steg.Text)
			if err != nil {
				uiSetStatus(status, err.Error(), true)
				return
			}
			var s strings.Builder
			s.WriteString(fmt.Sprintf("PSNR: %.2f dB\n", rep.PSNR))
			s.WriteString(fmt.Sprintf("SSIM: %.4f\n", rep.SSIM))
			for _, n := range rep.Notes {
				s.WriteString("  - " + n + "\n")
			}
			fyne.Do(func() {
				result.SetText(s.String())
				uiSetStatus(status, "评估完成", false)
			})
		}()
	})

	form := widget.NewForm(
		widget.NewFormItem("原图", container.NewBorder(nil, nil, nil, browseBtn(w, orig, false), orig)),
		widget.NewFormItem("隐写后图像", container.NewBorder(nil, nil, nil, browseBtn(w, steg, false), steg)),
	)
	card := uiCard("参数设置", form)
	actionBar := container.NewBorder(nil, nil, nil, runBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("质量 - 对比原图与隐写图的失真程度"),
		card,
		actionBar,
		uiCard("结果", result),
	)))
}

// =============================================================
// 自检审计页
// =============================================================

func newAuditTab(w fyne.Window) fyne.CanvasObject {
	input := widget.NewEntry()
	status := uiStatus("就绪")
	result := widget.NewMultiLineEntry()
	result.Disable()
	result.SetPlaceHolder("结果将显示在这里")

	runBtn := uiIconButton("执行自检", theme.InfoIcon(), func() {
		if input.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择图片"), w)
			return
		}
		uiSetStatus(status, "分析中...", false)
		go func() {
			res, err := sdk.AnalyzeImage(input.Text)
			if err != nil {
				uiSetStatus(status, err.Error(), true)
				return
			}
			text := fmt.Sprintf("判定: %s\n卡方检验: P=%.4f\nRS 分析 : 嵌入率≈%.1f%%\n",
				res.Verdict, res.ChiSquare, res.EmbedRate*100)
			for _, d := range res.Details {
				text += "  - " + d + "\n"
			}
			fyne.Do(func() {
				result.SetText(text)
				uiSetStatus(status, "分析完成", false)
			})
		}()
	})

	form := widget.NewForm(
		widget.NewFormItem("图片文件", container.NewBorder(nil, nil, nil, browseBtn(w, input, false), input)),
	)
	card := uiCard("参数设置", form)
	actionBar := container.NewBorder(nil, nil, nil, runBtn, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("自检审计 - 检测载体是否被篡改"),
		card,
		actionBar,
		uiCard("结果", result),
	)))
}

// =============================================================
// 批量页
// =============================================================

func newBatchTab(w fyne.Window) fyne.CanvasObject {
	dir := widget.NewEntry()
	secret := widget.NewEntry()
	output := widget.NewEntry()
	password := widget.NewPasswordEntry()
	algo := widget.NewSelect(sdk.Algorithms(), nil)
	algo.SetSelected("lsb")
	status := uiStatus("就绪")

	embedBtn := uiIconButton("批量嵌入", theme.ConfirmIcon(), func() {
		if dir.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择载体目录"), w)
			return
		}
		if secret.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择秘密文件"), w)
			return
		}
		if len(password.Text) == 0 {
			dialog.ShowError(fmt.Errorf("密码不能为空"), w)
			return
		}
		out := output.Text
		if out == "" {
			out = filepath.Join(dir.Text, "batch-out")
		}
		runAsync(w, status, func() error {
			res, err := sdk.BatchEmbed(sdk.BatchOptions{
				Options: sdk.Options{
					SecretPath: secret.Text,
					Password:   []byte(password.Text),
					Algorithm:  algo.Selected,
					BitDepth:   2,
				},
				InputDir:  dir.Text,
				OutputDir: out,
			})
			if err != nil {
				return err
			}
			var ok, fail int
			for _, r := range res {
				if r == nil {
					fail++
				} else {
					ok++
				}
			}
			showInfo(w, "完成", fmt.Sprintf("批量嵌入完成: 成功 %d, 失败 %d", ok, fail))
			return nil
		})
	})

	extractBtn := uiSecondaryButton("批量提取", theme.SearchIcon(), func() {
		if dir.Text == "" {
			dialog.ShowError(fmt.Errorf("请选择载体目录"), w)
			return
		}
		if len(password.Text) == 0 {
			dialog.ShowError(fmt.Errorf("密码不能为空"), w)
			return
		}
		out := output.Text
		if out == "" {
			out = filepath.Join(dir.Text, "batch-out")
		}
		runAsync(w, status, func() error {
			res, err := sdk.BatchExtract(sdk.BatchOptions{
				Options: sdk.Options{
					Password: []byte(password.Text),
				},
				InputDir:  dir.Text,
				OutputDir: out,
			})
			if err != nil {
				return err
			}
			var ok, fail int
			for _, r := range res {
				if r == nil {
					fail++
				} else {
					ok++
				}
			}
			showInfo(w, "完成", fmt.Sprintf("批量提取完成: 成功 %d, 失败 %d", ok, fail))
			return nil
		})
	})

	form := widget.NewForm(
		widget.NewFormItem("载体目录", container.NewBorder(nil, nil, nil, browseBtn(w, dir, true), dir)),
		widget.NewFormItem("秘密文件", container.NewBorder(nil, nil, nil, browseBtn(w, secret, false), secret)),
		widget.NewFormItem("输出目录", container.NewBorder(nil, nil, nil, browseBtn(w, output, true), output)),
		widget.NewFormItem("密码", password),
		widget.NewFormItem("隐写算法", algo),
	)
	buttons := container.NewHBox(embedBtn, extractBtn)
	actionBar := container.NewBorder(nil, nil, nil, buttons, status)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		sectionTitle("批量 - 目录级批量嵌入 / 提取"),
		uiCard("参数设置", form),
		actionBar,
	)))
}

// =============================================================
// 关于页
// =============================================================

func newAboutTab() fyne.CanvasObject {
	head := container.NewHBox(
		widget.NewIcon(theme.InfoIcon()),
		widget.NewLabelWithStyle("关于 StegGo", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	info := widget.NewLabel(`StegGo V2.0 - 抗检测隐写工具

核心链路: ZIP压缩 → 三因子密钥派生(PBKDF2) → AES-256-GCM → SHA256绑定
           → 六算法隐写(LSB/DCT/DWT/HUGO/WOW/UNIWARD) → 载体容器/套娃/Polyglot
自研壁垒: 确定性伪随机游走 + 成本加权嵌入 + 高斯噪声填充
自检审计: 卡方检验 / RS分析 / SPA分析
架构: 五层(common/crypto/algorithm/carrier/service) + 三层交互(CLI/TUI/GUI) + 离线铁则
GUI 驱动: steggo/pkg/steg/sdk 公开 SDK（与 CLI/TUI 功能统一）

支持载体:
  [图片] PNG/BMP/TIFF (JPG已拦截)
  [音频] WAV
  [文档] PDF (不破坏渲染结构)
  [文本] TXT/MD (零宽字符)
  [视频] 帧分片+XOR冗余

仅供学习与授权测试使用，禁止用于非法用途。`)
	info.Wrapping = fyne.TextWrapWord
	return container.NewVScroll(container.NewPadded(container.NewCenter(container.NewVBox(head, info))))
}
