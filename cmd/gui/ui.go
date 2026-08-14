package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// uiCard 创建带标题和浅边框的内容卡片。
func uiCard(title string, body fyne.CanvasObject) fyne.CanvasObject {
	label := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(
		container.NewVBox(label, widget.NewSeparator()), nil, nil, nil,
		container.NewPadded(body),
	)
}

// uiFormRow 创建左标签 + 右控件的表单行，标签宽度固定。
func uiFormRow(label string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, control)
}

// uiStatus 返回一个可显示状态/错误的多行只读区域。
// 用于替代状态栏 Label，避免长错误被截断。
func uiStatus(initial string) *widget.Entry {
	entry := widget.NewMultiLineEntry()
	entry.Disable()
	entry.SetText(initial)
	entry.Wrapping = fyne.TextWrapWord
	return entry
}

// uiSetStatus 设置状态区文本；isError 为 true 时前缀显示 [错误]。
func uiSetStatus(e *widget.Entry, msg string, isError bool) {
	s := msg
	if isError {
		s = "[错误] " + msg
	}
	fyne.Do(func() { e.SetText(s) })
}

// uiSuccessDialog 展示格式化成功信息（轻量替代 dialog.ShowInformation）。
func uiSuccessDialog(w fyne.Window, title string, msg string) {
	fyne.Do(func() {
		content := widget.NewLabel(msg)
		content.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustom(title, "好", container.NewPadded(content), w)
		d.Resize(fyne.NewSize(480, 320))
		d.Show()
	})
}

// uiErrorDialog 展示带滚动条的完整错误信息，避免长错误被截断。
func uiErrorDialog(w fyne.Window, err error) {
	fyne.Do(func() {
		if err == nil {
			return
		}
		entry := widget.NewMultiLineEntry()
		entry.Disable()
		entry.SetText(err.Error())
		entry.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustom("错误", "关闭", container.NewPadded(entry), w)
		d.Resize(fyne.NewSize(560, 360))
		d.Show()
	})
}

// uiIconButton 创建带图标的按钮（统一风格）。
func uiIconButton(label string, icon fyne.Resource, action func()) fyne.CanvasObject {
	btn := widget.NewButtonWithIcon(label, icon, action)
	btn.Importance = widget.HighImportance
	return btn
}

// uiSecondaryButton 创建次要的带图标按钮。
func uiSecondaryButton(label string, icon fyne.Resource, action func()) fyne.CanvasObject {
	btn := widget.NewButtonWithIcon(label, icon, action)
	btn.Importance = widget.MediumImportance
	return btn
}

// uiHint 创建一行提示文字。
func uiHint(s string) fyne.CanvasObject {
	lbl := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	lbl.Wrapping = fyne.TextWrapWord
	return lbl
}
