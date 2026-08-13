package algorithm

// init 注册内置算法。
func init() {
	Register(NewLSB())
	Register(NewDCT())
	Register(NewDWT())
	Register(NewHUGO())
	Register(NewWOW())
	Register(NewUNIWARD())
}
