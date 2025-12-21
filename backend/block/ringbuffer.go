package block

import (
	"sync"
)

type key struct {
	Height string
	Hash   string
}

// RingBuffer：固定长度区块缓存 + 去重集合
// - 长度固定 50（清单要求）
// - 重启清空（由上层初始化保证）
type RingBuffer struct {
	mu sync.RWMutex

	cap int
	buf []Block
	// newest first：buf[0] 最新
	seen map[key]struct{}
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 50
	}
	return &RingBuffer{
		cap:  capacity,
		buf:  make([]Block, 0, capacity),
		seen: make(map[key]struct{}, capacity),
	}
}

// AddIfNew：若 (height+hash) 未出现则插入顶部，返回 true；否则返回 false
func (r *RingBuffer) AddIfNew(b Block) bool {
	k := key{Height: b.Height, Hash: b.Hash}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.seen[k]; ok {
		return false
	}

	// 插入顶部（最新在前）
	r.buf = append([]Block{b}, r.buf...)
	r.seen[k] = struct{}{}

	// 超长：删除最旧
	for len(r.buf) > r.cap {
		last := r.buf[len(r.buf)-1]
		r.buf = r.buf[:len(r.buf)-1]
		delete(r.seen, key{Height: last.Height, Hash: last.Hash})
	}
	return true
}

// List：返回最新在前的副本
func (r *RingBuffer) List() []Block {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Block, 0, len(r.buf))
	out = append(out, r.buf...)
	return out
}

// Reset：清空（用于 205：切换判定规则后强制清空显示/去重）
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf = r.buf[:0]
	r.seen = make(map[key]struct{}, r.cap)
}
