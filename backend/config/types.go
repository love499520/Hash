package config

import (
	"tron-signal/backend/judge"
	"tron-signal/backend/machine"
	"tron-signal/backend/source"
)

// PollConfig：轮询失败策略（906）
// Auto=true：自动等待 N 分钟后再继续
// Auto=false：人工（即失败后停更；由 UI/重启/再次启用恢复）
type PollConfig struct {
	BaseTickMS  int  `json:"baseTickMS"`  // 全局最小 tick（调度节拍，建议 200~1000）
	Auto        bool `json:"auto"`        // 自动 or 人工
	WaitMinutes int  `json:"waitMinutes"` // 自动等待分钟
}

type Admin struct {
	Username string `json:"username"`
	// 为了避免引入 bcrypt 依赖，使用 sha256(salt+password)（足够测试机用）
	SaltHex string `json:"saltHex"`
	HashHex string `json:"hashHex"`
}

type StoreData struct {
	Version int `json:"version"`

	Admin Admin `json:"admin"`

	Tokens    []string `json:"tokens"`
	Whitelist []string `json:"whitelist"`

	JudgeRule judge.RuleType `json:"judgeRule"`

	Machines []machine.Config `json:"machines"`
	Sources  []source.Config  `json:"sources"`

	Poll PollConfig `json:"poll"`
}
