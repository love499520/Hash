package source

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"tron-signal/internal/block"
)

// AnkrREST
// 兼容 TRON 原生 REST 风格（如 getnowblock）
// 具体 URL 由配置提供，解析按 TRON 标准字段
type AnkrREST struct {
	id        string
	name      string
	endpoint  string
	headers   map[string]string
	enabled   bool
	baseRate  int
	maxRate   int
	lastCost  time.Duration
	client    *http.Client
}

func NewAnkrREST(id, name, endpoint string, headers map[string]string, enabled bool, baseRate, maxRate int) *AnkrREST {
	return &AnkrREST{
		id:       id,
		name:     name,
		endpoint: endpoint,
		headers:  headers,
		enabled:  enabled,
		baseRate: baseRate,
		maxRate:  maxRate,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (a *AnkrREST) ID() string            { return a.id }
func (a *AnkrREST) Name() string          { return a.name }
func (a *AnkrREST) Enabled() bool         { return a.enabled }
func (a *AnkrREST) BaseRate() int         { return a.baseRate }
func (a *AnkrREST) MaxRate() int          { return a.maxRate }
func (a *AnkrREST) LastLatency() time.Duration { return a.lastCost }

func (a *AnkrREST) MarkError(err error) {
	// 降频/熔断逻辑由 scheduler 统一处理
}

func (a *AnkrREST) FetchLatest(ctx context.Context) (*block.Meta, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", a.endpoint, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw struct {
		BlockID     string `json:"blockID"`
		BlockHeader struct {
			RawData struct {
				Number    int64 `json:"number"`
				Timestamp int64 `json:"timestamp"`
			} `json:"raw_data"`
		} `json:"block_header"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	if raw.BlockHeader.RawData.Number == 0 || raw.BlockID == "" {
		return nil, errors.New("invalid block data")
	}

	a.lastCost = time.Since(start)

	return &block.Meta{
		Height: raw.BlockHeader.RawData.Number,
		Hash:   raw.BlockID,
		Time:   time.Unix(raw.BlockHeader.RawData.Timestamp/1000, 0),
		Source: a.name,
	}, nil
}
