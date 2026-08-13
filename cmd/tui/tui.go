package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// =============================================================
// 简单状态机：menu → form → running → result → menu
// =============================================================

type state int

const (
	stateMenu state = iota
	stateForm
	stateRunning
	stateResult
)

// formSpec 描述一个表单。
type formSpec struct {
	title  string
	fields []fieldSpec
	run    func(values map[string]string) (string, error)
}

type fieldSpec struct {
	key      string
	label    string
	secret   bool // 密码型字段
	optional bool
}

type model struct {
	state     state
	cursor    int    // 菜单/字段光标
	selected  int    // 菜单选中项
	form      *formSpec
	inputs    []textinput.Model
	results   []string // 运行日志
	err       error
	running   bool
	finished  bool
	formDone  string // 运行完成后的显示文本
}

var menuItems = []string{
	"嵌入 (Hide) - 将秘密加密嵌入载体",
	"提取 (Extract) - 从载体还原秘密",
	"水印 (Watermark) - 公开版权标记",
	"自检审计 (Audit) - 卡方/RS/SPA 检测",
	"容量检测 (Capacity)",
	"质量评估 (Quality) - PSNR/SSIM",
	"退出",
}

// newMenuModel 创建初始菜单模型。
func newMenuModel() model {
	return model{state: stateMenu, cursor: 0}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateMenu:
		return m.updateMenu(msg)
	case stateForm:
		return m.updateForm(msg)
	case stateRunning:
		return m.updateRunning(msg)
	case stateResult:
		switch msg.(type) {
		case tea.KeyMsg:
			m.state = stateMenu
			m.cursor = 0
		}
		return m, nil
	}
	return m, nil
}

// =============================================================
// 菜单
// =============================================================

func (m model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "enter", " ":
		if m.cursor == len(menuItems)-1 {
			return m, tea.Quit
		}
		return m.activate(m.cursor), nil
	}
	return m, nil
}

// activate 根据菜单项创建对应表单。
func (m model) activate(idx int) tea.Model {
	switch idx {
	case 0:
		m.form = hideForm()
	case 1:
		m.form = extractForm()
	case 2:
		m.form = watermarkForm()
	case 3:
		m.form = auditForm()
	case 4:
		m.form = capacityForm()
	case 5:
		m.form = qualityForm()
	}
	if m.form == nil {
		return m
	}
	m.state = stateForm
	m.inputs = make([]textinput.Model, len(m.form.fields))
	for i, f := range m.form.fields {
		ti := textinput.New()
		ti.Placeholder = f.label
		ti.CharLimit = 200
		if f.secret {
			ti.EchoMode = textinput.EchoPassword
		}
		if i == 0 {
			ti.Focus()
		}
		m.inputs[i] = ti
	}
	m.cursor = 0
	return m
}

// =============================================================
// 表单
// =============================================================

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.inputs {
		var cmd tea.Cmd
		m.inputs[i], cmd = m.inputs[i].Update(msg)
		if i == m.cursor {
			cmds = append(cmds, cmd)
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, tea.Batch(cmds...)
	}
	switch key.String() {
	case "esc", "ctrl+c":
		m.state = stateMenu
		m.cursor = 0
		return m, nil
	case "tab", "down", "j":
		m.cursor = (m.cursor + 1) % len(m.inputs)
		m.syncFocus()
	case "shift+tab", "up", "k":
		m.cursor = (m.cursor - 1 + len(m.inputs)) % len(m.inputs)
		m.syncFocus()
	case "enter":
		// 提交表单
		values := make(map[string]string)
		ok := true
		for i, f := range m.form.fields {
			v := strings.TrimSpace(m.inputs[i].Value())
			if v == "" && !f.optional {
				ok = false
				break
			}
			values[f.key] = v
		}
		if !ok {
			return m, tea.Batch(append(cmds, tea.Println("⚠ 存在必填字段为空，请补全后回车提交"))...)
		}
		// 启动后台运行
		m.state = stateRunning
		m.results = nil
		m.err = nil
		return m, runInBackground(m.form.run, values)
	}
	return m, tea.Batch(cmds...)
}

// runInBackground 在后台执行表单逻辑。
func runInBackground(run func(map[string]string) (string, error), values map[string]string) tea.Cmd {
	return func() tea.Msg {
		text, err := run(values)
		return runDoneMsg{text: text, err: err}
	}
}

type runDoneMsg struct {
	text string
	err  error
}

func (m model) syncFocus() {
	for i := range m.inputs {
		if i == m.cursor {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m model) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	if done, ok := msg.(runDoneMsg); ok {
		m.state = stateResult
		if done.err != nil {
			m.formDone = "错误: " + done.err.Error()
		} else {
			m.formDone = done.text
		}
		return m, nil
	}
	return m, nil
}

// =============================================================
// 渲染
// =============================================================

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Padding(0, 1)
	menuStyle = lipgloss.NewStyle().Padding(0, 1)
	selStyle  = lipgloss.NewStyle().
			Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("0")).
			Padding(0, 1)
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func (m model) View() string {
	switch m.state {
	case stateMenu:
		return m.viewMenu()
	case stateForm:
		return m.viewForm()
	case stateRunning:
		return titleStyle.Render("⏳ 正在执行...") + "\n\n" + dimStyle.Render("请稍候，加密与隐写需要一点时间。")
	case stateResult:
		style := okStyle
		if m.formDone == "" || strings.HasPrefix(m.formDone, "错误:") {
			style = errStyle
		}
		return titleStyle.Render("结果") + "\n\n" + style.Render(m.formDone) + "\n\n" + dimStyle.Render("按任意键返回菜单")
	}
	return ""
}

func (m model) viewMenu() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("StegGo V2.0 TUI - 抗检测隐写工具") + "\n\n")
	for i, item := range menuItems {
		if i == m.cursor {
			sb.WriteString(selStyle.Render("▶ "+item) + "\n")
		} else {
			sb.WriteString(menuStyle.Render("  "+item) + "\n")
		}
	}
	sb.WriteString("\n" + dimStyle.Render("↑/↓ 选择   Enter 确认   q 退出"))
	return sb.String()
}

func (m model) viewForm() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(m.form.title) + "\n\n")
	for i, f := range m.form.fields {
		label := f.label
		if m.cursor == i {
			label = selStyle.Render(label)
		}
		sb.WriteString(label + "  " + m.inputs[i].View() + "\n")
	}
	sb.WriteString("\n" + dimStyle.Render("Tab/↑↓ 切换字段   Enter 提交   Esc 返回"))
	return sb.String()
}
