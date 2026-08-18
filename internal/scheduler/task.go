// Package scheduler 任务调度解析器：解析 TXT/CSV 任务清单并批量执行隐写/提取。
package scheduler

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Task 一条隐写/提取任务。
type Task struct {
	Action    string // hide / extract / sg-create / sg-open
	Carrier   string // 载体文件路径
	Secret    string // 秘密文件路径（hide）
	Output    string // 输出路径/目录
	Password  string // 密码（明文仅存在于任务清单，执行后由调用方清理）
	Algorithm string // 算法（可选，默认自动）
	BitDepth  int    // 位深（可选）
	UseSM4    bool   // 是否 SM4 国密
	Line      int    // 清单中的行号（用于错误定位）
}

// Summary 批量执行汇总。
type Summary struct {
	Total   int
	Success int
	Failed  int
	Errors  []string // 每条失败的描述（含行号）
}

// ParseFile 解析任务清单（按扩展名自动识别 CSV / TXT）。
func ParseFile(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取任务清单: %w", err)
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".csv") {
		return ParseCSV(data)
	}
	return ParseTXT(data)
}

// ParseCSV 解析 CSV 任务清单。
// 首行表头：action,carrier,secret,password,algorithm,bits,sm4,output
func ParseCSV(data []byte) ([]Task, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	if len(records) < 2 {
		return nil, errors.New("CSV 任务清单为空（需要表头 + 至少一行任务）")
	}
	header := make(map[string]int)
	for i, h := range records[0] {
		header[strings.ToLower(strings.TrimSpace(h))] = i
	}
	need := []string{"action", "carrier"}
	for _, n := range need {
		if _, ok := header[n]; !ok {
			return nil, fmt.Errorf("CSV 缺少必需列: %s", n)
		}
	}
	get := func(row []string, name string) string {
		i, ok := header[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var tasks []Task
	for ln, row := range records[1:] {
		action := get(row, "action")
		if action == "" {
			continue // 跳过空行
		}
		t := Task{
			Action:    strings.ToLower(action),
			Carrier:   get(row, "carrier"),
			Secret:    get(row, "secret"),
			Output:    get(row, "output"),
			Password:  get(row, "password"),
			Algorithm: get(row, "algorithm"),
			Line:      ln + 2, // 1-based，跳过表头
		}
		if b := get(row, "bits"); b != "" {
			t.BitDepth, _ = strconv.Atoi(b)
		}
		if s := get(row, "sm4"); s != "" {
			t.UseSM4, _ = strconv.ParseBool(s)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ParseTXT 解析 TXT 任务清单。
// 每行格式（键值对，以 | 或空格分隔，值含空格用双引号）：
//
//	action=hide carrier="cover.png" secret="msg.txt" password="pass" algorithm=lsb bits=2 sm4=true output="out.steg"
//
// 也兼容简写：hide cover.png msg.txt pass
func ParseTXT(data []byte) ([]Task, error) {
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var tasks []Task
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields, err := splitKV(line)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", ln, err)
		}
		action := strings.ToLower(fields["action"])
		if action == "" {
			// 兼容简写：第一个 token 是动作
			action = strings.ToLower(fields["_0"])
		}
		if action == "" {
			return nil, fmt.Errorf("第 %d 行: 缺少 action", ln)
		}
		t := Task{
			Action:    action,
			Carrier:   firstNonEmpty(fields["carrier"], fields["_1"]),
			Secret:    firstNonEmpty(fields["secret"], fields["_2"]),
			Password:  fields["password"],
			Output:    fields["output"],
			Algorithm: fields["algorithm"],
			Line:      ln,
		}
		if b := fields["bits"]; b != "" {
			t.BitDepth, _ = strconv.Atoi(b)
		}
		if s := fields["sm4"]; s != "" {
			t.UseSM4, _ = strconv.ParseBool(s)
		}
		tasks = append(tasks, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, errors.New("TXT 任务清单为空（需要至少一行任务）")
	}
	return tasks, nil
}

// splitKV 解析一行键值对；位置参数记录为 _0, _1, ...
// 支持引号包裹的值（含空格），分隔符为空格或竖线。
func splitKV(line string) (map[string]string, error) {
	// 分词：支持双引号
	var toks []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '|') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()

	m := make(map[string]string)
	pos := 0
	for _, tk := range toks {
		if strings.Contains(tk, "=") {
			kv := strings.SplitN(tk, "=", 2)
			m[kv[0]] = strings.Trim(kv[1], "\"")
		} else {
			m["_"+strconv.Itoa(pos)] = strings.Trim(tk, "\"")
			pos++
		}
	}
	return m, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
