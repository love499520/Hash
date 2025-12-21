package judge

import (
	"strings"
	"unicode"

	"tron-signal/backend/machine"
)

// RuleType 判定规则类型
type RuleType string

const (
	Lucky RuleType = "lucky" // 幸运
	Big   RuleType = "big"   // 大小
	Odd   RuleType = "odd"   // 单双
)

// Judge 全局判定器
// 任一时刻只能有一种规则生效
type Judge struct {
	current RuleType
}

// New 创建判定器，默认幸运规则
func New() *Judge {
	return &Judge{current: Lucky}
}

// SetRule 设置新规则（外部需负责二次确认 + reset 状态机）
func (j *Judge) SetRule(r RuleType) {
	j.current = r
}

// GetRule 获取当前规则
func (j *Judge) GetRule() RuleType {
	return j.current
}

// Decide 根据 hash 判定 ON / OFF
func (j *Judge) Decide(hash string) machine.State {
	switch j.current {
	case Big:
		return judgeBig(hash)
	case Odd:
		return judgeOdd(hash)
	default:
		return judgeLucky(hash)
	}
}

// === 具体规则实现 ===

// 幸运规则
// hash 最后两位
// 字母+数字 / 数字+字母 → ON
// 字母+字母 / 数字+数字 → OFF
func judgeLucky(hash string) machine.State {
	h := lastN(hash, 2)
	if len(h) < 2 {
		return machine.OFF
	}

	a, b := rune(h[0]), rune(h[1])

	if (isLetter(a) && isDigit(b)) || (isDigit(a) && isLetter(b)) {
		return machine.ON
	}
	return machine.OFF
}

// 大小规则
// hash 最后一个数字（忽略字母）
// 0–4 → ON，5–9 → OFF
func judgeBig(hash string) machine.State {
	d, ok := lastDigit(hash)
	if !ok {
		return machine.OFF
	}
	if d <= 4 {
		return machine.ON
	}
	return machine.OFF
}

// 单双规则
// hash 最后一个数字
// 偶数 → ON，奇数 → OFF
func judgeOdd(hash string) machine.State {
	d, ok := lastDigit(hash)
	if !ok {
		return machine.OFF
	}
	if d%2 == 0 {
		return machine.ON
	}
	return machine.OFF
}

// === 工具函数 ===

func lastN(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}

func lastDigit(s string) (int, bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if unicode.IsDigit(rune(s[i])) {
			return int(s[i] - '0'), true
		}
	}
	return 0, false
}

func isLetter(r rune) bool {
	return unicode.IsLetter(r)
}

func isDigit(r rune) bool {
	return unicode.IsDigit(r)
}
