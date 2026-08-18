package scheduler

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const sampleCSV = `action,carrier,secret,password,algorithm,bits,sm4,output
hide,cover.png,msg.txt,pass123,lsb,2,false,out1.steg
extract,out1.steg,,pass123,dct,,false,out_dir
sg-create,cover.png,,sgpass,,,true,cover.png.sg
`

const sampleTXT = `# 批量任务清单
action=hide carrier="cover one.png" secret=msg.txt password=pass123 algorithm=lsb bits=2 sm4=false output="out 1.steg"
action=extract carrier=out1.steg password=pass123 output=out_dir
action=sg-create carrier=cover.png password=sgpass sm4=true output=cover.png.sg
`

func TestParseCSV(t *testing.T) {
	tasks, err := ParseCSV([]byte(sampleCSV))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("任务数应为 3，实际 %d", len(tasks))
	}
	if tasks[0].Action != "hide" || tasks[0].Carrier != "cover.png" || tasks[0].Password != "pass123" ||
		tasks[0].Algorithm != "lsb" || tasks[0].BitDepth != 2 || tasks[0].UseSM4 != false {
		t.Fatalf("任务 0 解析错误: %+v", tasks[0])
	}
	if tasks[1].Action != "extract" || tasks[1].Carrier != "out1.steg" || tasks[1].BitDepth != 0 {
		t.Fatalf("任务 1 解析错误: %+v", tasks[1])
	}
	if tasks[2].UseSM4 != true {
		t.Fatalf("任务 2 sm4 解析错误: %+v", tasks[2])
	}
	if tasks[0].Line != 2 || tasks[2].Line != 4 {
		t.Fatalf("行号错误: %d %d", tasks[0].Line, tasks[2].Line)
	}
}

func TestParseTXT(t *testing.T) {
	tasks, err := ParseTXT([]byte(sampleTXT))
	if err != nil {
		t.Fatalf("ParseTXT: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("任务数应为 3，实际 %d", len(tasks))
	}
	if tasks[0].Carrier != "cover one.png" {
		t.Fatalf("带引号含空格路径解析错误: %q", tasks[0].Carrier)
	}
	if tasks[0].Output != "out 1.steg" || tasks[0].BitDepth != 2 || tasks[0].UseSM4 != false {
		t.Fatalf("任务 0 解析错误: %+v", tasks[0])
	}
	if tasks[2].UseSM4 != true {
		t.Fatalf("任务 2 sm4 错误: %+v", tasks[2])
	}
}

func TestParseFileByExtension(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "tasks.csv")
	txtPath := filepath.Join(dir, "tasks.txt")
	if err := os.WriteFile(csvPath, []byte(sampleCSV), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtPath, []byte(sampleTXT), 0o600); err != nil {
		t.Fatal(err)
	}
	ct, err := ParseFile(csvPath)
	if err != nil || len(ct) != 3 {
		t.Fatalf("CSV 文件解析失败: %v", err)
	}
	tt, err := ParseFile(txtPath)
	if err != nil || len(tt) != 3 {
		t.Fatalf("TXT 文件解析失败: %v", err)
	}
}

// TestRunnerExecute 真实执行一次 hide + extract 任务链。
func TestRunnerExecute(t *testing.T) {
	dir := t.TempDir()
	carrier := filepath.Join(dir, "cover.png")
	secret := filepath.Join(dir, "msg.txt")
	if err := writePNG(carrier, 64, 64); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("scheduler-roundtrip"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.steg")
	extractDir := filepath.Join(dir, "out")

	tasks := []Task{
		{Action: "hide", Carrier: carrier, Secret: secret, Output: out, Password: "pw", Algorithm: "lsb", BitDepth: 1, Line: 1},
		{Action: "extract", Carrier: out, Output: extractDir, Password: "pw", Line: 2},
	}
	sum := (&Runner{}).Run(tasks)
	if sum.Failed != 0 {
		t.Fatalf("执行失败: %v", sum.Errors)
	}
	if sum.Success != 2 {
		t.Fatalf("成功数应为 2，实际 %d", sum.Success)
	}
	// 验证提取出的文件
	got, err := os.ReadFile(filepath.Join(extractDir, "msg.txt"))
	if err != nil {
		t.Fatalf("提取产物缺失: %v", err)
	}
	if string(got) != "scheduler-roundtrip" {
		t.Fatalf("提取内容不一致: %q", got)
	}
}

func TestRunnerEmptyPassword(t *testing.T) {
	sum := (&Runner{}).Run([]Task{{Action: "hide", Carrier: "a.png", Secret: "b.txt", Line: 1}})
	if sum.Success != 0 || sum.Failed != 1 {
		t.Fatalf("空密码任务应失败: %+v", sum)
	}
}

// writePNG 生成指定尺寸的纯色 PNG 载体。
func writePNG(path string, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
