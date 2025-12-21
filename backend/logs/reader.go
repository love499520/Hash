package logs

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Time  string `json:"time"`
	Level string `json:"level"` // INFO / WARN / ERROR / MAJOR
	Text  string `json:"text"`
	File  string `json:"file"`
}

// 简单规则：包含 MAJOR_ 前缀 / ABNORMAL_RESTART / MAJOR_BLOCK_GAP 认为重大
func classify(line string) string {
	u := strings.ToUpper(line)
	if strings.Contains(u, "MAJOR_") || strings.Contains(u, "ABNORMAL_RESTART") || strings.Contains(u, "MAJOR_BLOCK_GAP") {
		return "MAJOR"
	}
	if strings.Contains(u, "ERROR") {
		return "ERROR"
	}
	if strings.Contains(u, "WARN") {
		return "WARN"
	}
	return "INFO"
}

// ListLogFiles：按日期排序
func ListLogFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".log") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files, nil
}

// TailRead：从最新文件开始倒序读取，聚合到 limit 行
func TailRead(dir string, limit int, levelFilter string, majorOnly bool) ([]Entry, error) {
	if limit <= 0 {
		limit = 200
	}
	files, err := ListLogFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no_logs")
	}

	var out []Entry
	// 从最新到最旧
	for i := len(files) - 1; i >= 0 && len(out) < limit; i-- {
		f := files[i]
		lines, _ := readAllLines(f)
		// 从尾部往前
		for j := len(lines) - 1; j >= 0 && len(out) < limit; j-- {
			line := strings.TrimSpace(lines[j])
			if line == "" {
				continue
			}
			lv := classify(line)
			if majorOnly && lv != "MAJOR" {
				continue
			}
			if levelFilter != "" && strings.ToUpper(levelFilter) != lv {
				continue
			}
			out = append(out, Entry{
				Time:  guessTime(line),
				Level: lv,
				Text:  line,
				File:  filepath.Base(f),
			})
		}
	}
	return out, nil
}

// readAllLines：简单读取（日志量不大时足够；后续可换 mmap）
func readAllLines(path string) ([]string, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	var lines []string
	sc := bufio.NewScanner(fp)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, nil
}

// guessTime：尽量从行首提取 YYYY/MM/DD HH:mm:ss 或 2025/12/..
// 否则返回当前时间（不影响过滤）
func guessTime(line string) string {
	// 你权威日志可能是：2025/12/19 16:25:57.922814 ...
	// UI 要秒级：YYYY/MM/DD HH:mm:ss
	if len(line) >= 19 {
		p := line[:19]
		// 简单校验：包含 / 和 :
		if strings.Count(p, "/") == 2 && strings.Count(p, ":") >= 2 {
			return p
		}
	}
	return time.Now().Format("2006/01/02 15:04:05")
}
