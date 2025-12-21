package source

import (
	"context"
	"time"
)

// Block 统一的区块数据结构
type Block struct {
	Height string // 使用 string，避免不同源精度/格式差异
	Hash   string
	Time   time.Time
	Source string
}

// Config 单个数据源配置
type Config struct {
	ID string

	// 请求端点
	Endpoint string
	Headers  map[string]string

	// 轮询阈值
	BaseRate int // 基础频率（次/秒）
	MaxRate  int // 上限频率（次/秒）

	Enabled bool
}

// Fetcher 数据源接口（HTTP）
type Fetcher interface {
	ID() string
	Config() *Config
	FetchLatest(ctx context.Context) (*Block, error)
}

// Result 并发抓取返回
type Result struct {
	Block *Block
	Err   error
	From  string
}
