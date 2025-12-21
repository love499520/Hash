package judge

import (
	"strings"
	"unicode"
)

// RuleType 判定规则类型
type RuleType string

const (
	RuleLucky RuleType = "lucky" // 幸运
	RuleSize  RuleType = "size"  // 大小
	RuleOdd   RuleType = "odd"   // 单双
)

// Engine 全局判定引擎（只允许一个生效）
type Engine struct {
	current RuleType
}

// NewEngine 创建判定引擎，默认幸运规则
func NewEngine() *Engine {
	return &Engine{
		current: RuleLucky,
	}
}

// Current 返回当前生效规则
func (e *Engine) Current() RuleType {
	return e.current
}

// Switch 切换判定规则（外部需先停状态机并确认）
// 不做任何隐式副作用
func (e *Engine) Switch(rule RuleType) {
	e.current = rule
}

// Judge 根据当前规则判断 ON / OFF
func (e *Engine) Judge(hash string) bool {
	switch e.current {
	case RuleLucky:
		return judgeLucky(hash)
	case RuleSize:
		return judgeSize(hash)
	case RuleOdd:
		return judgeOdd(hash)
	default:
		return false
	}
}

// ---------- 具体规则实现 ----------

// 幸运规则
// hash 最后两位：
// 字母+数字 / 数字+字母 → ON
// 字母+字母 / 数字+数字 → OFF
func judgeLucky(hash string) bool {
	if len(hash) < 2 {
		return false
	}
	last := hash[len(hash)-2:]

	a := rune(last[0])
	b := rune(last[1])

	aIsDigit := unicode.IsDigit(a)
	bIsDigit := unicode.IsDigit(b)

	// 一字母一数字
	return aIsDigit != bIsDigit
}

// 大小规则
// hash 最后一个数字（忽略字母）
// 0–4 → ON，5–9 → OFF
func judgeSize(hash string) bool {
	for i := len(hash) - 1; i >= 0; i-- {
		if hash[i] >= '0' && hash[i] <= '9' {
			return hash[i] <= '4'
		}
	}
	return false
}

// 单双规则
// hash 最后一个数字
// 偶数 → ON，奇数 → OFF
func judgeOdd(hash string) bool {
	for i := len(hash) - 1; i >= 0; i-- {
		if hash[i] >= '0' && hash[i] <= '9' {
			return ((hash[i]-'0')%2 == 0)
		}
	}
	return false
}

// Label 返回规则中文名称（用于 UI）
func Label(rule RuleType) string {
	switch rule {
	case RuleLucky:
		return "幸运"
	case RuleSize:
		return "大小"
	case RuleOdd:
		return "单双"
	default:
		return strings.ToUpper(string(rule))
	}
}
