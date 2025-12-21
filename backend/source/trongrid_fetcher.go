package source

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// TronGridFetcher
// 典型 endpoint: https://api.trongrid.io/wallet/getnowblock
// headers 可包含 TRON-PRO-API-KEY 等
type TronGridFetcher struct {
	cfg     Config
	limiter *Limiter
	client  *http.Client
}

func NewTronGridFetcher(cfg Config) *TronGridFetcher {
	return &TronGridFetcher{
		cfg:     cfg,
		limiter: NewLimiter(cfg.BaseRate, cfg.MaxRate),
		client: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

func (t *TronGridFetcher) ID() string       { return t.cfg.ID }
func (t *TronGridFetcher) Config() *Config  { return &t.cfg }

func (t *TronGridFetcher) UpdateConfig(cfg Config) {
	t.cfg = cfg
	t.limiter.Update(cfg.BaseRate, cfg.MaxRate)
}

func (t *TronGridFetcher) FetchLatest(ctx context.Context) (*Block, error) {
	if !t.cfg.Enabled {
		return nil, errors.New("disabled")
	}
	if !t.limiter.Allow() {
		return nil, errors.New("rate_limited")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", t.cfg.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// TronGrid getnowblock 格式
	var raw struct {
		BlockID     string `json:"blockID"`
		BlockHeader struct {
			RawData struct {
				Number    int64 `json:"number"`
				Timestamp int64 `json:"timestamp"` // ms
			} `json:"raw_data"`
		} `json:"block_header"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if raw.BlockID == "" || raw.BlockHeader.RawData.Number <= 0 {
		return nil, errors.New("invalid_block")
	}

	return &Block{
		Height: itoa64(raw.BlockHeader.RawData.Number),
		Hash:   raw.BlockID,
		Time:   time.Unix(raw.BlockHeader.RawData.Timestamp/1000, 0),
		Source: "trongrid",
	}, nil
}
