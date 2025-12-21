package machine

import "time"

// State 表示区块判定状态
type State bool

const (
	OFF State = false
	ON  State = true
)

// Config 状态机配置（持久化）
type Config struct {
	ID string

	// 触发规则
	TriggerState State
	TriggerCount int

	// HIT 规则
	HitEnabled bool
	HitExpect  State
	HitOffset  int // T+X

	Enabled bool
}

// Runtime 状态机运行态（不持久化）
type Runtime struct {
	Count          int
	WaitingReverse bool
	BaseHeight     int64
	HitWaiting     bool
	HitTarget      int64
}

// Signal 触发信号
type Signal struct {
	MachineID string
	Type      string // TRIGGER / HIT
	Height    int64
	Time      time.Time
}

// Machine 状态机实例
type Machine struct {
	Config  Config
	Runtime Runtime
}

// New 创建状态机（运行态强制清零）
func New(cfg Config) *Machine {
	return &Machine{
		Config: cfg,
		Runtime: Runtime{
			Count:          0,
			WaitingReverse: true,
			HitWaiting:     false,
		},
	}
}

// ResetRuntime 切换规则 / 停机时调用
func (m *Machine) ResetRuntime() {
	m.Runtime = Runtime{
		Count:          0,
		WaitingReverse: true,
		HitWaiting:     false,
	}
}

// Process 处理一个新区块
func (m *Machine) Process(
	height int64,
	state State,
	now time.Time,
) *Signal {

	if !m.Config.Enabled {
		return nil
	}

	// === HIT 等待阶段 ===
	if m.Runtime.HitWaiting {
		if height == m.Runtime.HitTarget {
			m.Runtime.HitWaiting = false
			if state == m.Config.HitExpect {
				return &Signal{
					MachineID: m.Config.ID,
					Type:      "HIT",
					Height:    height,
					Time:      now,
				}
			}
		}
		// HIT 只观察一次
		if height >= m.Runtime.HitTarget {
			m.Runtime.HitWaiting = false
		}
		return nil
	}

	// === 等待反向阶段 ===
	if m.Runtime.WaitingReverse {
		if state != m.Config.TriggerState {
			m.Runtime.WaitingReverse = false
			m.Runtime.Count = 0
		}
		return nil
	}

	// === 计数阶段 ===
	if state == m.Config.TriggerState {
		m.Runtime.Count++
	} else {
		m.Runtime.Count = 0
	}

	// === 触发条件 ===
	if m.Runtime.Count >= m.Config.TriggerCount {
		m.Runtime.Count = 0
		m.Runtime.WaitingReverse = true

		sig := &Signal{
			MachineID: m.Config.ID,
			Type:      "TRIGGER",
			Height:    height,
			Time:      now,
		}

		// HIT 规则（仅在触发后立即附加）
		if m.Config.HitEnabled && m.Config.HitOffset > 0 {
			m.Runtime.HitWaiting = true
			m.Runtime.HitTarget = height + int64(m.Config.HitOffset)
		}

		return sig
	}

	return nil
}
