package source

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"tron-signal/internal/block"
)

// Ankr REST API（TRON 原生 REST）
// 示例接口：/v1/tron/block/latest
type AnkrREST struct {
	id       string
	name     string
	endpoint string
	apiKey   string
	enabled  bool
	baseRate int
	maxRate  int
	lastCost time.Duration
	client   *http.Client
}

func NewAnkrREST(id, endpoint, apiKey string, enabled bool, baseRate, maxRate int) *AnkrREST {
	return &AnkrREST{
		id:       id,
		name:     "ankr-rest",
		endpoint: endpoint,
		apiKey:   apiKey,
		enabled:  enabled,
		baseRate: baseRate,
		maxRate:  maxRate,
		client: &http.Client{
			Timeout: 6 * time.Second,
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
	// 限流/熔断交由 scheduler
}

func (a *AnkrREST) FetchLatest(ctx context.Context) (*block.Meta, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", a.endpoint, nil)
	if err != nil {
		return nil, err
	}
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw struct {
		BlockID   string `json:"blockID"`
		BlockTime int64  `json:"blockTime"`
		Height    int64  `json:"height"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	if raw.Height == 0 || raw.BlockID == "" {
		return nil, errors.New("invalid block from ankr rest")
	}

	a.lastCost = time.Since(start)

	return &block.Meta{
		Height: raw.Height,
		Hash:   raw.BlockID,
		Time:   time.Unix(raw.BlockTime/1000, 0),
		Source: a.name,
	}, nil
}
