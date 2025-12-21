package source

import (
	"context"
	"time"

	"tron-signal/internal/block"
)

// Source
// HTTP 数据源统一接口（Ankr RPC / Ankr REST / TronGrid 等）
//
// 约束：
// - 只做 HTTP 轮询
// - 返回“最新有效区块元信息”
// - 不做任何状态机 / 判定逻辑
type Source interface {
	// ID 唯一标识
	ID() string

	// Name 用于日志/UI展示
	Name() string

	// Enabled 是否启用
	Enabled() bool

	// FetchLatest
	// 拉取最新区块元信息
	// 若无新区块，返回 (nil, nil)
	FetchLatest(ctx context.Context) (*block.Meta, error)

	// BaseRate 基础轮询频率（每秒）
	BaseRate() int

	// MaxRate 上限频率（每秒）
	MaxRate() int

	// MarkError 记录一次错误（用于限流/降频）
	MarkError(err error)

	// LastLatency 最近一次请求耗时
	LastLatency() time.Duration
}
