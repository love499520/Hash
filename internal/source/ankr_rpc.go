package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tron-signal/internal/block"
)

type AnkrRPC struct {
	id        string
	name      string
	endpoint  string
	enabled   bool
	baseRate  int
	maxRate   int
	lastCost  time.Duration
	client    *http.Client
}

func NewAnkrRPC(id, name, endpoint string, enabled bool, baseRate, maxRate int) *AnkrRPC {
	return &AnkrRPC{
		id:       id,
		name:     name,
		endpoint: endpoint,
		enabled:  enabled,
		baseRate: baseRate,
		maxRate:  maxRate,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (a *AnkrRPC) ID() string        { return a.id }
func (a *AnkrRPC) Name() string      { return a.name }
func (a *AnkrRPC) Enabled() bool     { return a.enabled }
func (a *AnkrRPC) BaseRate() int     { return a.baseRate }
func (a *AnkrRPC) MaxRate() int      { return a.maxRate }
func (a *AnkrRPC) LastLatency() time.Duration { return a.lastCost }

func (a *AnkrRPC) MarkError(err error) {
	// 这里仅记录，降频逻辑由 scheduler/limiter 统一处理
}

type rpcReq struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type rpcResp struct {
	Result string `json:"result"`
	Error  any    `json:"error"`
}

func (a *AnkrRPC) FetchLatest(ctx context.Context) (*block.Meta, error) {
	start := time.Now()

	// step 1: eth_blockNumber
	req1 := rpcReq{
		JSONRPC: "2.0",
		Method:  "eth_blockNumber",
		Params:  []any{},
		ID:      1,
	}
	b1, _ := json.Marshal(req1)
	r1, err := a.do(ctx, b1)
	if err != nil {
		return nil, err
	}

	heightHex := strings.TrimPrefix(r1.Result, "0x")
	height, err := strconv.ParseInt(heightHex, 16, 64)
	if err != nil {
		return nil, err
	}

	// step 2: eth_getBlockByNumber
	req2 := rpcReq{
		JSONRPC: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{r1.Result, false},
		ID:      2,
	}
	b2, _ := json.Marshal(req2)
	r2raw, err := a.doRaw(ctx, b2)
	if err != nil {
		return nil, err
	}

	var blockResp struct {
		Result struct {
			Hash      string `json:"hash"`
			Timestamp string `json:"timestamp"`
		} `json:"result"`
	}
	if err := json.Unmarshal(r2raw, &blockResp); err != nil {
		return nil, err
	}

	tsHex := strings.TrimPrefix(blockResp.Result.Timestamp, "0x")
	ts, _ := strconv.ParseInt(tsHex, 16, 64)

	a.lastCost = time.Since(start)

	return &block.Meta{
		Height: height,
		Hash:   blockResp.Result.Hash,
		Time:   time.Unix(ts, 0),
		Source: a.name,
	}, nil
}

func (a *AnkrRPC) do(ctx context.Context, body []byte) (*rpcResp, error) {
	raw, err := a.doRaw(ctx, body)
	if err != nil {
		return nil, err
	}
	var r rpcResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, errors.New("rpc error")
	}
	return &r, nil
}

func (a *AnkrRPC) doRaw(ctx context.Context, body []byte) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", a.endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioReadAll(resp)
}

// 独立封装，避免直接用 ioutil
func ioReadAll(r *http.Response) ([]byte, error) {
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}
