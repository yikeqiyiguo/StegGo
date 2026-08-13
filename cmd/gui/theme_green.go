// 淡绿色亮色主题
package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// greenTheme 基于 LightTheme 的淡绿色自定义主题，仅覆盖颜色。
type greenTheme struct {
	fyne.Theme
}

func newGreenTheme() fyne.Theme {
	return &greenTheme{Theme: theme.LightTheme()}
}

func (t *greenTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0xEB, G: 0xF5, B: 0xEA, A: 0xFF} // 主背景：淡绿白
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0x1E, G: 0x45, B: 0x2B, A: 0xFF} // 文字：深绿
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x2E, G: 0x8B, B: 0x57, A: 0xFF} // 强调：海绿
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // 主色按钮文字：白
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x3D, G: 0x9E, B: 0x5F, A: 0xFF} // 按钮：绿
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x66, G: 0xBB, B: 0x6A, A: 0x3D} // 悬停：半透明绿
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x2E, G: 0x8B, B: 0x57, A: 0x99} // 焦点边框
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // 输入框：白
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x6E, G: 0x8F, B: 0x76, A: 0xFF} // 占位符：灰绿
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0xAF, G: 0xC5, B: 0xB4, A: 0xFF} // 禁用文字
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0xC8, G: 0xE6, B: 0xC9, A: 0xFF} // 分隔线：浅绿
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xA8, G: 0xDE, B: 0xAD, A: 0xFF} // 选中背景
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0xF2, G: 0xFA, B: 0xEF, A: 0xFF} // 菜单背景
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0xEB, G: 0xF5, B: 0xEA, A: 0xE8} // 弹窗背景
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0x2E, G: 0x8B, B: 0x57, A: 0x66} // 滚动条
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x2E, G: 0x8B, B: 0x57, A: 0xFF}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0xDD, G: 0xF0, B: 0xDC, A: 0xFF} // 页头背景：浅绿
	}
	return t.Theme.Color(name, theme.VariantLight)
}
