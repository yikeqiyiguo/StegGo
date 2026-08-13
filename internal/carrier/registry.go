package carrier

// init 注册内置载体实现（插件化：外部可通过 Register 扩展）。
func init() {
	Register(&imageCarrier{})
	Register(wavCarrier)
	Register(pdfCarrier)
	Register(videoCarrier)
	Register(&textCarrier{})
}
