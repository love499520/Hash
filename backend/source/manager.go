package source

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Block 是统一的区块抽象结构
// 所有数据源必须转换为该结构
type Block struct {
	Height    int64
	Hash      string
	Timestamp time.Time
	Source    string
}

// Source 接口：所有 HTTP 数据源必须实现
type Source interface {
	Name() string
	Enabled() bool
	SetEnabled(bool)

	BaseRate() int // 基础阈值（每秒）
	MaxRate() int  // 上限阈值（每秒）

	FetchLatestBlock(ctx context.Context) (*Block, error)
}

// Manager 负责多数据源调度（先到先用）
type Manager struct {
	mu      sync.RWMutex
	sources []Source
}

// NewManager 创建数据源管理器
func NewManager() *Manager {
	return &Manager{
		sources: make([]Source, 0),
	}
}

// Register 注册一个数据源
func (m *Manager) Register(src Source) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources = append(m.sources, src)
}

// List 返回当前所有数据源（用于管理 UI）
func (m *Manager) List() []Source {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Source, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, s)
	}
	return out
}

// FetchFirstAvailable
// 并发请求所有 Enabled 的数据源
// 谁先返回有效新区块就用谁
func (m *Manager) FetchFirstAvailable(ctx context.Context) (*Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.sources) == 0 {
		return nil, errors.New("no data source registered")
	}

	type result struct {
		block *Block
		err   error
	}

	ch := make(chan result, len(m.sources))

	active := 0
	for _, src := range m.sources {
		if !src.Enabled() {
			continue
		}
		active++

		go func(s Source) {
			b, err := s.FetchLatestBlock(ctx)
			ch <- result{block: b, err: err}
		}(src)
	}

	if active == 0 {
		return nil, errors.New("no enabled data source")
	}

	// 谁先成功用谁
	for i := 0; i < active; i++ {
		select {
		case r := <-ch:
			if r.err == nil && r.block != nil {
				return r.block, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, errors.New("all data sources failed")
}
