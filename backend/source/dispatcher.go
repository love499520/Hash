package source

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// Fetcher：所有数据源统一接口
type Fetcher interface {
	ID() string
	Config() *Config
	UpdateConfig(cfg Config)
	FetchLatest(ctx context.Context) (*Block, error)
}

// Dispatcher：并发多源，先到先用
// 规则：
// - 启用的源并发拉取
// - 最先返回“有效 block”的即采用
// - rate_limited / disabled / invalid_block 将被跳过（并记录日志）
// - 全部失败则返回 error
type Dispatcher struct {
	mu       sync.RWMutex
	fetchers []Fetcher
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		fetchers: []Fetcher{},
	}
}

func (d *Dispatcher) Add(f Fetcher) {
	if f == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fetchers = append(d.fetchers, f)
}

// ReplaceAll：用于 UI 保存数据源后热更新（顺序与启停状态由 cfg 控制）
func (d *Dispatcher) ReplaceAll(list []Fetcher) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fetchers = list
}

func (d *Dispatcher) snapshot() []Fetcher {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Fetcher, 0, len(d.fetchers))
	out = append(out, d.fetchers...)
	return out
}

// FetchAny：并发拉取，先到先用
func (d *Dispatcher) FetchAny(ctx context.Context) (*Block, error) {
	fs := d.snapshot()
	if len(fs) == 0 {
		return nil, errors.New("no_sources")
	}

	type result struct {
		b   *Block
		err error
		id  string
	}

	// 子 ctx（避免某个源卡住）
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, len(fs))
	var wg sync.WaitGroup

	for _, f := range fs {
		f := f
		if f == nil {
			continue
		}
		cfg := f.Config()
		if cfg == nil || !cfg.Enabled {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			start := time.Now()
			b, err := f.FetchLatest(cctx)
			cost := time.Since(start)

			// 统一日志
			if err != nil {
				// 909：达到上限或限流，需要记录日志并降频/跳过（这里先记录；降频由 limiter 自身完成 + UI 可调整）
				// 1003：源不可用必须记录日志
				log.Printf("SOURCE_ERR id=%s type=%s err=%v cost=%s\n", cfg.ID, cfg.Type, err, cost)
				ch <- result{b: nil, err: err, id: cfg.ID}
				return
			}

			// 成功也可记录（可按需关掉）
			log.Printf("SOURCE_OK id=%s type=%s height=%s cost=%s\n", cfg.ID, cfg.Type, b.Height, cost)
			ch <- result{b: b, err: nil, id: cfg.ID}
		}()
	}

	// 关闭协程收尾
	go func() {
		wg.Wait()
		close(ch)
	}()

	// “先到先用”：收到第一个有效 block 立即 cancel 其他
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ctx.Err()
		case r, ok := <-ch:
			if !ok {
				if lastErr == nil {
					lastErr = errors.New("all_sources_failed")
				}
				return nil, lastErr
			}

			// 跳过典型无效错误
			if r.err != nil {
				lastErr = r.err
				continue
			}
			if r.b == nil || r.b.Hash == "" || r.b.Height == "" {
				lastErr = errors.New("invalid_block")
				continue
			}

			// 命中：先到先用
			cancel()
			return r.b, nil
		}
	}
}
