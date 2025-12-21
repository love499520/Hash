package machine

import (
	"sync"
	"time"
)

// Manager 管理多个状态机实例
type Manager struct {
	mu       sync.RWMutex
	machines map[string]*Machine
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		machines: make(map[string]*Machine),
	}
}

// Add 添加 / 覆盖一个状态机
func (m *Manager) Add(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.machines[cfg.ID] = New(cfg)
}

// Remove 删除状态机
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.machines, id)
}

// ListConfigs 返回所有状态机配置（用于 UI）
func (m *Manager) ListConfigs() []Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Config, 0, len(m.machines))
	for _, mc := range m.machines {
		out = append(out, mc.Config)
	}
	return out
}

// ResetAllRuntime
// - 判定规则切换
// - 全局停机
// - 强制清零
func (m *Manager) ResetAllRuntime() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mc := range m.machines {
		mc.ResetRuntime()
	}
}

// ProcessBlock
// - 对所有状态机处理一个新区块
// - 返回所有产生的信号（可能为 0 / 多个）
func (m *Manager) ProcessBlock(
	height int64,
	state State,
	now time.Time,
) []*Signal {

	m.mu.RLock()
	defer m.mu.RUnlock()

	var signals []*Signal
	for _, mc := range m.machines {
		if sig := mc.Process(height, state, now); sig != nil {
			signals = append(signals, sig)
		}
	}
	return signals
}
