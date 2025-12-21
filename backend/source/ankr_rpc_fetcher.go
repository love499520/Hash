package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// AnkrRpcFetcher
// 使用 JSON-RPC 接口作为数据源（HTTP POST）
// endpoint + headers 由用户配置
//
// 说明：不同 TRON JSON-RPC 网关 method 可能不同，
// 这里采用“可配置 method + params”的方式，避免写死。
// 默认你可以配置为：{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}
// 但 TRON 可能使用不同 method，需要你在 UI 配置里提供。
// —— 这符合你“用户自行配置与调度”的封板原则。
type AnkrRpcFetcher struct {
	cfg     Config
	limiter *Limiter
	client  *http.Client

	// JSON-RPC 请求模板（可配置）
	method string
	params any
}

func NewAnkrRpcFetcher(cfg Config, method string, params any) *AnkrRpcFetcher {
	return &AnkrRpcFetcher{
		cfg:     cfg,
		limiter: NewLimiter(cfg.BaseRate, cfg.MaxRate),
		client: &http.Client{
			Timeout: 6 * time.Second,
		},
		method: method,
		params: params,
	}
}

func (a *AnkrRpcFetcher) ID() string       { return a.cfg.ID }
func (a *AnkrRpcFetcher) Config() *Config  { return &a.cfg }

func (a *AnkrRpcFetcher) UpdateConfig(cfg Config) {
	a.cfg = cfg
	a.limiter.Update(cfg.BaseRate, cfg.MaxRate)
}

// FetchLatest
// JSON-RPC 不一定能直接给你 hash+time，常见是返回 blockNumber 或 block object。
// 为了与你清单一致（必须拿到 hash），这里设计为：
// 1) 先调用 method 获取最新高度（hex 或 int）
// 2) 再调用第二个 method 获取该高度对应 block（返回 hash / timestamp）
//
// 如果你的 Ankr TRON JSON-RPC 网关只支持一步返回 block，
// 也可以把 method 配成“直接返回 block”的方式（见 response parse）。
func (a *AnkrRpcFetcher) FetchLatest(ctx context.Context) (*Block, error) {
	if !a.cfg.Enabled {
		return nil, errors.New("disabled")
	}
	if !a.limiter.Allow() {
		return nil, errors.New("rate_limited")
	}

	// step1: get latest height (or block)
	req1 := rpcReq{JSONRPC: "2.0", ID: 1, Method: a.method, Params: a.params}
	raw1, err := a.post(ctx, req1)
	if err != nil {
		return nil, err
	}

	// 尝试解析成两种返回：
	// A) 直接返回 block object
	if blk, ok := parseBlockObject(raw1); ok {
		return blk, nil
	}
	// B) 返回 height（hex string 或 number）
	h, ok := parseHeight(raw1)
	if !ok {
		return nil, errors.New("rpc_invalid_height")
	}

	// step2: get block by height (常见 method: eth_getBlockByNumber / getblockbynum 等)
	// 这里不写死方法名：通过 cfg.Headers 中的 X-RPC-BLOCK-METHOD 指定（保持简单）
	blockMethod := a.cfg.Headers["X-RPC-BLOCK-METHOD"]
	if blockMethod == "" {
		// 不提供则无法继续拿 hash
		return nil, errors.New("rpc_missing_block_method")
	}

	req2 := rpcReq{JSONRPC: "2.0", ID: 2, Method: blockMethod, Params: []any{h, false}}
	raw2, err := a.post(ctx, req2)
	if err != nil {
		return nil, err
	}

	blk, ok := parseBlockObject(raw2)
	if !ok {
		return nil, errors.New("rpc_invalid_block")
	}
	return blk, nil
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

func (a *AnkrRpcFetcher) post(ctx context.Context, req rpcReq) (map[string]any, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	for k, v := range a.cfg.Headers {
		// 注意：X-RPC-BLOCK-METHOD 是内部约定，不应作为 HTTP Header 发送
		if k == "X-RPC-BLOCK-METHOD" {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if raw["error"] != nil {
		return raw, errors.New("rpc_error")
	}
	return raw, nil
}

// parseHeight 支持 hex string / number
func parseHeight(raw map[string]any) (any, bool) {
	v, ok := raw["result"]
	if !ok || v == nil {
		return nil, false
	}
	// 直接返回高度（eth_blockNumber -> "0x..."）
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		// JSON number
		return int64(t), true
	default:
		return nil, false
	}
}

// parseBlockObject 尝试从 result 中解析出 block 字段（hash + number + timestamp）
func parseBlockObject(raw map[string]any) (*Block, bool) {
	v, ok := raw["result"]
	if !ok || v == nil {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}

	// 常见字段：hash / number / timestamp
	hash, _ := obj["hash"].(string)
	if hash == "" {
		// TRON 某些网关可能叫 blockID
		hash, _ = obj["blockID"].(string)
	}
	if hash == "" {
		return nil, false
	}

	// number: hex or number
	height := ""
	switch n := obj["number"].(type) {
	case string:
		height = hexToDecString(n)
	case float64:
		height = itoa64(int64(n))
	default:
		// TRON 可能叫 "block_header.raw_data.number"
		height = ""
	}

	// timestamp: hex / number / ms
	var ts time.Time
	switch t := obj["timestamp"].(type) {
	case string:
		// hex timestamp
		ts = time.Unix(hexToInt64(t), 0)
	case float64:
		// 可能是 ms
		val := int64(t)
		if val > 2_000_000_000_000 {
			ts = time.Unix(val/1000, 0)
		} else {
			ts = time.Unix(val, 0)
		}
	default:
		ts = time.Now()
	}

	if height == "" {
		// 最差兜底：不返回高度也能用 hash，但你的系统还需要高度做跳跃检测
		// 这里选择失败，要求源返回高度
		return nil, false
	}

	return &Block{
		Height: height,
		Hash:   hash,
		Time:   ts,
		Source: "ankr-rpc",
	}, true
}

// hexToDecString 将 "0x..." 转十进制字符串（最小实现）
func hexToDecString(h string) string {
	return itoa64(hexToInt64(h))
}

func hexToInt64(h string) int64 {
	if len(h) >= 2 && (h[0:2] == "0x" || h[0:2] == "0X") {
		h = h[2:]
	}
	var v int64
	for i := 0; i < len(h); i++ {
		c := h[i]
		var n byte
		switch {
		case c >= '0' && c <= '9':
			n = c - '0'
		case c >= 'a' && c <= 'f':
			n = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			n = c - 'A' + 10
		default:
			return 0
		}
		v = v*16 + int64(n)
	}
	return v
}
