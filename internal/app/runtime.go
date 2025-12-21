package app

import (
	"sync"
	"time"
)

// Runtime
// 全局运行态（仅内存，启动必清零）
// 用于 /api/status、SSE 状态流
type Runtime struct {
	mu sync.RWMutex

	Listening      bool
	LastHeight     int64
	LastHash       string
	LastTime       time.Time
	Reconnects     int64
	ConnectedKeys  int
	JudgeRuleName  string
}

// NewRuntime 创建运行态
func NewRuntime() *Runtime {
	return &Runtime{}
}

// Reset
// 启动时强制清空所有运行态
func (r *Runtime) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Listening = false
	r.LastHeight = 0
	r.LastHash = ""
	r.LastTime = time.Time{}
	r.Reconnects = 0
	r.ConnectedKeys = 0
	r.JudgeRuleName = ""
}

// UpdateBlock
// 更新最近区块信息
func (r *Runtime) UpdateBlock(height int64, hash string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.LastHeight = height
	r.LastHash = hash
	r.LastTime = t
}

// Snapshot
// 返回运行态快照（只读）
func (r *Runtime) Snapshot() Runtime {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return Runtime{
		Listening:     r.Listening,
		LastHeight:    r.LastHeight,
		LastHash:      r.LastHash,
		LastTime:      r.LastTime,
		Reconnects:    r.Reconnects,
		ConnectedKeys: r.ConnectedKeys,
		JudgeRuleName: r.JudgeRuleName,
	}
}
