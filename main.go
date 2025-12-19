package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/*
	Tron 实时区块监听与交易信号系统（落地版）
	- 纯标准库：无第三方依赖
	- Web 管理台：首次 setup + login
	- API Key 管理：最多 3 个，热更新
	- 规则：ON/OFF 阈值（滑块）；HIT：t+x（x 可配）+ expect
	- 区块来源：轮询 Tron Fullnode /wallet/getnowblock（可后续替换为 TronGrid WS）
	- 去重：RingBuffer(50) on (height+hash)
	- ON/OFF 判定：hash 最后两位 “字母/数字 类型异或”
	- 状态机：waitingReverse（触发后需先见反向状态才能重新计数）
	- 信号广播：/ws 服务器端 WS 广播（不缓存、不重试、不确认）
	- SSE：/sse/status 推最新块信息给页面
	- 重启：运行态强制清零（不恢复任何历史状态）
*/

const (
	listenAddr   = ":8080"
	dataDir      = "data"
	configPath   = "data/config.json"
	logDir       = "logs"
	logRetention = 3 // days

	ringSize = 50

	// 轮询间隔：为了“实时”，默认 1s
	pollInterval = 1 * time.Second

	// Tron Fullnode API（可用 TronGrid 公共网关）
	defaultNodeURL = "https://api.trongrid.io"
)

// ---------- Config / Models ----------

type Config struct {
	Web WebCred `json:"web"`

	APIKeys []string `json:"apiKeys"`

	// JudgeRule: LUCKY | BIGSMALL | ODDEVEN
	JudgeRule string `json:"judgeRule"`

	Rules Rules `json:"rules"`

	Access AccessControl `json:"access"`
}

type WebCred struct {
	Initialized bool   `json:"initialized"`
	Username    string `json:"username"`
	SaltHex     string `json:"saltHex"`
	HashHex     string `json:"hashHex"`
}

type AccessControl struct {
	IPWhitelist []string          `json:"ipWhitelist"`
	Tokens      map[string]uint64 `json:"tokens"` // token -> usage count
}

type Rules struct {
	On  ThresholdRule `json:"on"`
	Off ThresholdRule `json:"off"`
	Hit HitRule       `json:"hit"`
}

type ThresholdRule struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold"` // 0-20; 0 means never trigger
}

type HitRule struct {
	Enabled bool   `json:"enabled"`
	Expect  string `json:"expect"` // "ON" or "OFF"
	Offset  int    `json:"offset"` // x, >=1
}

type Status struct {
	Listening     bool   `json:"listening"`
	LastHeight    int64  `json:"lastHeight"`
	LastHash      string `json:"lastHash"`
	LastTimeISO   string `json:"lastTimeISO"`
	Reconnects    uint64 `json:"reconnects"`
	ConnectedKeys int    `json:"connectedKeys"`
}

// Signal broadcast to trading program
type Signal struct {
	Type       string `json:"type"`       // "ON"|"OFF"|"HIT"
	Height     int64  `json:"height"`     // current block height (trigger/hit block)
	BaseHeight int64  `json:"baseHeight"` // trigger base height (for HIT: trigger base)
	State      string `json:"state"`      // "ON"|"OFF" (for HIT: the state observed at t+x)
	TimeISO    string `json:"time"`       // ISO timestamp
}

// ---------- Globals (runtime state must be reset every boot) ----------

var (
	cfgMu sync.RWMutex
	cfg   Config

	// sessions: token -> username
	sessMu   sync.Mutex
	sessions = map[string]string{}

	// runtime: forced reset every start
	rtMu sync.Mutex
	rt   RuntimeState

	// ws clients (broadcast)
	wsMu      sync.Mutex
	wsClients = map[*wsConn]struct{}{}

	// sse subscribers
	sseMu   sync.Mutex
	sseSubs = map[chan Status]struct{}{}

	// logger
	logger *log.Logger
)

type RuntimeState struct {
	// counters
	OnCounter  int
	OffCounter int

	// state machine
	WaitingReverse bool
	LastTriggered  string // "ON"|"OFF" (empty at start)
	BaseHeight     int64

	// hit waiting
	HitWaiting   bool
	HitBase      int64
	HitOffset    int
	HitExpect    string // "ON"|"OFF"
	HitArmedTime time.Time

	// ring buffer (height+hash)
	Ring ringBuffer

	// last status
	LastHeight int64
	LastHash   string
	LastTime   time.Time

	// listening
	Listening bool
}

type ringBuffer struct {
	buf   [ringSize]string
	idx   int
	full  bool
	index map[string]struct{}
}

func (r *ringBuffer) reset() {
	r.idx = 0
	r.full = false
	r.index = make(map[string]struct{}, ringSize)
	for i := 0; i < ringSize; i++ {
		r.buf[i] = ""
	}
}

func (r *ringBuffer) has(key string) bool {
	_, ok := r.index[key]
	return ok
}

func (r *ringBuffer) add(key string) {
	if r.index == nil {
		r.reset()
	}
	// if slot occupied, delete old
	old := r.buf[r.idx]
	if old != "" {
		delete(r.index, old)
	}
	r.buf[r.idx] = key
	r.index[key] = struct{}{}

	r.idx++
	if r.idx >= ringSize {
		r.idx = 0
		r.full = true
	}
}

// ---------- Utilities ----------

func ensureDirs() error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	return nil
}

func rotateLogs() (*os.File, error) {
	// log file per day: logs/YYYY-MM-DD.log
	name := time.Now().Format("2006-01-02") + ".log"
	path := filepath.Join(logDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// cleanup old logs
	entries, err := os.ReadDir(logDir)
	if err == nil {
		cutoff := time.Now().AddDate(0, 0, -logRetention)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			// parse date prefix
			fn := e.Name()
			if !strings.HasSuffix(fn, ".log") || len(fn) < len("2006-01-02.log") {
				continue
			}
			ds := strings.TrimSuffix(fn, ".log")
			t, parseErr := time.Parse("2006-01-02", ds)
			if parseErr != nil {
				continue
			}
			if t.Before(cutoff) {
				_ = os.Remove(filepath.Join(logDir, fn))
			}
		}
	}

	return f, nil
}

func loadConfig() (Config, error) {
	var c Config
	b, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// defaults
			c.Access.Tokens = map[string]uint64{}
			c.JudgeRule = "LUCKY"
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Access.Tokens == nil {
		c.Access.Tokens = map[string]uint64{}
	}
	// default judge rule
	if !isValidJudgeRule(c.JudgeRule) {
		c.JudgeRule = "LUCKY"
	}
	return c, nil
}

func saveConfigLocked(c Config) error {
	tmp := configPath + ".tmp"
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func randHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func mustJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

// ---------- Auth ----------

func isLoggedIn(r *http.Request) bool {
	c, err := r.Cookie("TSID")
	if err != nil || c.Value == "" {
		return false
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	_, ok := sessions[c.Value]
	return ok
}

func requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfgMu.RLock()
		initialized := cfg.Web.Initialized
		cfgMu.RUnlock()
		if !initialized {
			// force setup
			if r.URL.Path != "/setup" && r.URL.Path != "/api/setup" {
				http.Redirect(w, r, "/setup", http.StatusFound)
				return
			}
			next(w, r)
			return
		}
		if !isLoggedIn(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func setupPage(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	initialized := cfg.Web.Initialized
	cfgMu.RUnlock()
	if initialized {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><title>Setup</title>
<style>body{font-family:system-ui;padding:24px;max-width:480px;margin:auto}input{width:100%;padding:10px;margin:8px 0}button{padding:10px 14px}</style>
</head><body>
<h2>首次设置账号密码</h2>
<p>设置完成后才能进入系统。</p>
<form method="post" action="/api/setup">
<label>用户名</label><input name="u" required>
<label>密码</label><input name="p" type="password" required>
<button type="submit">保存</button>
</form>
</body></html>`)
}

func setupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	u := strings.TrimSpace(r.FormValue("u"))
	p := r.FormValue("p")
	if u == "" || p == "" {
		http.Error(w, "username/password required", http.StatusBadRequest)
		return
	}

	salt, err := randHex(16)
	if err != nil {
		http.Error(w, "rand failed", http.StatusInternalServerError)
		return
	}
	hash := sha256Hex(salt + ":" + p)

	cfgMu.Lock()
	defer cfgMu.Unlock()
	if cfg.Web.Initialized {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	cfg.Web = WebCred{
		Initialized: true,
		Username:    u,
		SaltHex:     salt,
		HashHex:     hash,
	}
	if err := saveConfigLocked(cfg); err != nil {
		http.Error(w, "save config failed", http.StatusInternalServerError)
		return
	}
	logger.Println("SYSTEM_SETUP_DONE")
	http.Redirect(w, r, "/login", http.StatusFound)
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	initialized := cfg.Web.Initialized
	cfgMu.RUnlock()
	if !initialized {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><title>Login</title>
<style>body{font-family:system-ui;padding:24px;max-width:480px;margin:auto}input{width:100%;padding:10px;margin:8px 0}button{padding:10px 14px}</style>
</head><body>
<h2>登录</h2>
<form method="post" action="/api/login">
<label>用户名</label><input name="u" required>
<label>密码</label><input name="p" type="password" required>
<button type="submit">登录</button>
</form>
</body></html>`)
}
func externalGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// internal pages should already be protected by login; this is for external APIs if you want
		cfgMu.RLock()
		whitelist := append([]string(nil), cfg.Access.IPWhitelist...)
		tokens := cfg.Access.Tokens
		cfgMu.RUnlock()

		ip := clientIP(r.RemoteAddr)
		if ipAllowed(ip, whitelist) {
			next(w, r)
			return
		}

		// external must have token
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-Token")
		}
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		cfgMu.Lock()
		defer cfgMu.Unlock()
		if _, ok := tokens[token]; !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		tokens[token]++
		cfg.Access.Tokens = tokens
		_ = saveConfigLocked(cfg)

		next(w, r)
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func ipAllowed(ip string, whitelist []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, entry := range whitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// CIDR or single IP
		if strings.Contains(entry, "/") {
			_, netw, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if netw.Contains(parsed) {
				return true
			}
		} else {
			if parsed.Equal(net.ParseIP(entry)) {
				return true
			}
		}
	}
	return false
}

func loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	u := strings.TrimSpace(r.FormValue("u"))
	p := r.FormValue("p")

	cfgMu.RLock()
	web := cfg.Web
	cfgMu.RUnlock()

	if !web.Initialized {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	if u != web.Username {
		http.Error(w, "invalid", http.StatusUnauthorized)
		return
	}
	hash := sha256Hex(web.SaltHex + ":" + p)
	if hash != web.HashHex {
		http.Error(w, "invalid", http.StatusUnauthorized)
		return
	}

	sid, _ := randHex(16)
	sessMu.Lock()
	sessions[sid] = u
	sessMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "TSID",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("TSID")
	if c != nil {
		sessMu.Lock()
		delete(sessions, c.Value)
		sessMu.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:     "TSID",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ---------- Pages / Static ----------

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("web", "index.html"))
}

// ---------- APIs ----------

func apiStatus(w http.ResponseWriter, r *http.Request) {
	rtMu.Lock()
	st := Status{
		Listening:   rt.Listening,
		LastHeight:  rt.LastHeight,
		LastHash:    rt.LastHash,
		LastTimeISO: isoOrEmpty(rt.LastTime),
		Reconnects:  0,
	}
	rtMu.Unlock()

	cfgMu.RLock()
	st.ConnectedKeys = len(cfg.APIKeys)
	cfgMu.RUnlock()

	mustJSON(w, 200, st)
}

func apiGetAPIKeys(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	keys := append([]string(nil), cfg.APIKeys...)
	cfgMu.RUnlock()
	mustJSON(w, 200, map[string]any{"apiKeys": keys})
}

func apiSetAPIKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKeys []string `json:"apiKeys"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	keys := make([]string, 0, len(req.APIKeys))
	for _, k := range req.APIKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) > 3 {
		keys = keys[:3]
	}

	cfgMu.Lock()
	cfg.APIKeys = keys
	_ = saveConfigLocked(cfg)
	cfgMu.Unlock()

	// start/stop listener depending on keys count
	if len(keys) >= 1 {
		tryStartListener()
	} else {
		stopListener()
	}

	logger.Printf("APIKEY_UPDATED count=%d", len(keys))
	mustJSON(w, 200, map[string]any{"apiKeys": keys})
}

func apiGetRules(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	rules := cfg.Rules
	cfgMu.RUnlock()
	mustJSON(w, 200, rules)
}

func apiSetRules(w http.ResponseWriter, r *http.Request) {
	var in Rules
	if err := readJSON(r, &in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// sanitize
	in.On.Threshold = clamp(in.On.Threshold, 0, 20)
	in.Off.Threshold = clamp(in.Off.Threshold, 0, 20)

	in.Hit.Offset = clamp(in.Hit.Offset, 1, 20)
	in.Hit.Expect = strings.ToUpper(strings.TrimSpace(in.Hit.Expect))
	if in.Hit.Expect != "ON" && in.Hit.Expect != "OFF" {
		in.Hit.Expect = "ON"
	}

	cfgMu.Lock()
	cfg.Rules = in
	_ = saveConfigLocked(cfg)
	cfgMu.Unlock()

	logger.Printf("RULES_UPDATED on=%v/%d off=%v/%d hit=%v/x=%d/%s",
		in.On.Enabled, in.On.Threshold,
		in.Off.Enabled, in.Off.Threshold,
		in.Hit.Enabled, in.Hit.Offset, in.Hit.Expect)

	mustJSON(w, 200, map[string]any{"ok": true})
}

func isValidJudgeRule(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "LUCKY", "BIGSMALL", "ODDEVEN":
		return true
	default:
		return false
	}
}

func apiGetJudge(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	rule := cfg.JudgeRule
	cfgMu.RUnlock()
	if rule == "" {
		rule = "LUCKY"
	}
	mustJSON(w, 200, map[string]any{"rule": rule})
}

func disableAllMachinesAndResetRuntimeLocked() {
	// Stop all state machines: in this simplified single-machine version, that means disabling ON/OFF/HIT rules.
	cfgMu.Lock()
	cfg.Rules.On.Enabled = false
	cfg.Rules.Off.Enabled = false
	cfg.Rules.Hit.Enabled = false
	_ = saveConfigLocked(cfg)
	cfgMu.Unlock()

	// Reset runtime counters/state
	rtMu.Lock()
	rt.OnCounter = 0
	rt.OffCounter = 0
	rt.WaitingReverse = true
	rt.LastTriggered = ""
	rt.BaseHeight = 0
	rt.HitWaiting = false
	rt.HitBase = 0
	rt.HitOffset = 0
	rt.HitExpect = ""
	rt.HitArmedTime = time.Time{}
	rtMu.Unlock()
}

func apiSetJudge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule    string `json:"rule"`
		Confirm bool   `json:"confirm"`
		AckStop bool   `json:"ackStop"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	rule := strings.ToUpper(strings.TrimSpace(req.Rule))
	if !isValidJudgeRule(rule) {
		http.Error(w, "invalid rule", http.StatusBadRequest)
		return
	}
	if !req.Confirm || !req.AckStop {
		http.Error(w, "need confirm", http.StatusBadRequest)
		return
	}

	cfgMu.Lock()
	from := cfg.JudgeRule
	if from == "" {
		from = "LUCKY"
	}
	if from == rule {
		cfgMu.Unlock()
		mustJSON(w, 200, map[string]any{"ok": true, "rule": from})
		return
	}
	cfg.JudgeRule = rule
	_ = saveConfigLocked(cfg)
	cfgMu.Unlock()

	disableAllMachinesAndResetRuntimeLocked()

	logger.Printf("SEV=MAJOR EVT=JUDGE_RULE_CHANGED FROM=%s TO=%s machinesStopped=true countersCleared=true", from, rule)
	mustJSON(w, 200, map[string]any{"ok": true, "rule": rule})
}

// ---------- SSE ----------

func sseStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Status, 8)
	sseMu.Lock()
	sseSubs[ch] = struct{}{}
	sseMu.Unlock()

	defer func() {
		sseMu.Lock()
		delete(sseSubs, ch)
		sseMu.Unlock()
	}()

	// initial push
	rtMu.Lock()
	init := Status{
		Listening:   rt.Listening,
		LastHeight:  rt.LastHeight,
		LastHash:    rt.LastHash,
		LastTimeISO: isoOrEmpty(rt.LastTime),
	}
	rtMu.Unlock()
	_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", mustMarshalJSON(init))
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-ch:
			_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", mustMarshalJSON(st))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func mustMarshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func pushSSE(st Status) {
	sseMu.Lock()
	defer sseMu.Unlock()
	for ch := range sseSubs {
		select {
		case ch <- st:
		default:
		}
	}
}

// ---------- Listener (polling) ----------

var (
	listenMu  sync.Mutex
	listenCtx context.Context
	listenCan context.CancelFunc
)

func tryStartListener() {
	cfgMu.RLock()
	keysCount := len(cfg.APIKeys)
	cfgMu.RUnlock()
	if keysCount < 1 {
		return
	}

	listenMu.Lock()
	defer listenMu.Unlock()
	if listenCan != nil {
		return // already running
	}

	ctx, cancel := context.WithCancel(context.Background())
	listenCtx = ctx
	listenCan = cancel

	rtMu.Lock()
	rt.Listening = true
	rtMu.Unlock()

	go listenerLoop(ctx)
	logger.Println("LISTENER_START")
}

func stopListener() {
	listenMu.Lock()
	defer listenMu.Unlock()
	if listenCan != nil {
		listenCan()
		listenCan = nil
	}

	rtMu.Lock()
	rt.Listening = false
	rtMu.Unlock()

	logger.Println("LISTENER_STOP")
}

func listenerLoop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfgMu.RLock()
			keys := append([]string(nil), cfg.APIKeys...)
			cfgMu.RUnlock()
			if len(keys) < 1 {
				stopListener()
				return
			}
			// simple round robin by time
			idx := int(time.Now().UnixNano()) % len(keys)
			apiKey := keys[idx]

			height, hash, blockTime, err := fetchNowBlock(apiKey)
			if err != nil {
				logger.Printf("POLL_ERROR: %v", err)
				continue
			}

			rtMu.Lock()
			rt.LastHeight = height
			rt.LastHash = hash
			rt.LastTime = blockTime
			st := Status{
				Listening:   rt.Listening,
				LastHeight:  rt.LastHeight,
				LastHash:    rt.LastHash,
				LastTimeISO: isoOrEmpty(rt.LastTime),
			}
			rtMu.Unlock()
			pushSSE(st)

			// process block
			processBlock(height, hash, blockTime)
		}
	}
}

func fetchNowBlock(apiKey string) (int64, string, time.Time, error) {
	// POST /wallet/getnowblock
	url := defaultNodeURL + "/wallet/getnowblock"

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", apiKey)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return 0, "", time.Time{}, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var out struct {
		BlockID     string `json:"blockID"`
		BlockHeader struct {
			RawData struct {
				Number    int64 `json:"number"`
				Timestamp int64 `json:"timestamp"`
			} `json:"raw_data"`
		} `json:"block_header"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := dec.Decode(&out); err != nil {
		return 0, "", time.Time{}, err
	}
	if out.BlockID == "" || out.BlockHeader.RawData.Number == 0 {
		return 0, "", time.Time{}, errors.New("bad block data")
	}

	bt := time.UnixMilli(out.BlockHeader.RawData.Timestamp).UTC()
	return out.BlockHeader.RawData.Number, out.BlockID, bt, nil
}
// ---------- Core: dedup + judge + state machine ----------

func processBlock(height int64, hash string, blockTime time.Time) {
	// dedup
	key := fmt.Sprintf("%d:%s", height, hash)

	rtMu.Lock()
	if rt.Ring.index == nil {
		rt.Ring.reset()
	}
	if rt.Ring.has(key) {
		rtMu.Unlock()
		return
	}
	rt.Ring.add(key)
	rtMu.Unlock()

	// judge ON/OFF
	state, ok := judgeState(hash)
	if !ok {
		logger.Printf("JUDGE_FAIL height=%d hash=%s", height, hash)
		return
	}

	cfgMu.RLock()
	rules := cfg.Rules
	cfgMu.RUnlock()

	// state machine
	signals := runStateMachine(height, state, blockTime, rules)

	// broadcast signals
	for _, s := range signals {
		broadcastSignal(s)
	}
}

func judgeState(hash string) (string, bool) {
	cfgMu.RLock()
	rule := cfg.JudgeRule
	cfgMu.RUnlock()
	rule = strings.ToUpper(strings.TrimSpace(rule))
	if rule == "" {
		rule = "LUCKY"
	}

	switch rule {
	case "BIGSMALL":
		// last digit ignoring letters: 0-4 ON / 5-9 OFF
		d, ok := lastDigit(hash)
		if !ok {
			logger.Printf("JUDGE_NO_DIGIT rule=BIGSMALL hash=%s", shrinkHash(hash))
			return "", false
		}
		if d <= '4' {
			return "ON", true
		}
		return "OFF", true

	case "ODDEVEN":
		// last digit ignoring letters: even ON / odd OFF
		d, ok := lastDigit(hash)
		if !ok {
			logger.Printf("JUDGE_NO_DIGIT rule=ODDEVEN hash=%s", shrinkHash(hash))
			return "", false
		}
		if ((d - '0') % 2) == 0 {
			return "ON", true
		}
		return "OFF", true

	case "LUCKY":
		fallthrough
	default:
		// original: last two chars XOR type(letter/digit)
		h := strings.ToLower(strings.TrimSpace(hash))
		if len(h) < 2 {
			return "", false
		}
		a := h[len(h)-2]
		b := h[len(h)-1]
		ta, oka := charType(a)
		tb, okb := charType(b)
		if !oka || !okb {
			return "", false
		}
		if ta != tb {
			return "ON", true
		}
		return "OFF", true
	}
}

func shrinkHash(h string) string {
	h = strings.TrimSpace(h)
	if len(h) <= 13 {
		return h
	}
	return h[:5] + "***" + h[len(h)-5:]
}

func lastDigit(hash string) (byte, bool) {
	h := strings.TrimSpace(hash)
	for i := len(h) - 1; i >= 0; i-- {
		c := h[i]
		if c >= '0' && c <= '9' {
			return c, true
		}
	}
	return 0, false
}

func charType(c byte) (string, bool) {
	switch {
	case c >= '0' && c <= '9':
		return "D", true
	case c >= 'a' && c <= 'f':
		return "A", true
	default:
		return "", false
	}
}

func runStateMachine(height int64, state string, t time.Time, rules Rules) []Signal {
	rtMu.Lock()
	defer rtMu.Unlock()

	out := make([]Signal, 0, 2)

	// HIT check first: only observe t+x once
	if rt.HitWaiting && height == rt.HitBase+int64(rt.HitOffset) {
		rt.HitWaiting = false
		if state == rt.HitExpect {
			s := Signal{
				Type:       "HIT",
				Height:     height,
				BaseHeight: rt.HitBase,
				State:      state,
				TimeISO:    t.UTC().Format(time.RFC3339Nano),
			}
			out = append(out, s)
			logger.Printf("HIT_SIGNAL base=%d height=%d state=%s", rt.HitBase, height, state)
		} else {
			logger.Printf("HIT_MISS base=%d height=%d got=%s expect=%s", rt.HitBase, height, state, rt.HitExpect)
		}
	}

	// waitingReverse logic
	if rt.WaitingReverse {
		if rt.LastTriggered == "" {
			// initial: allow counting immediately
			rt.WaitingReverse = false
		} else {
			rev := reverseOf(rt.LastTriggered)
			if state == rev {
				rt.WaitingReverse = false
				// reset counters when reverse met
				rt.OnCounter = 0
				rt.OffCounter = 0
			} else {
				return out
			}
		}
	}

	// counting per target rule
	// Note: In this simplified version, we treat ON rule and OFF rule as two independent triggers,
	// but only one can logically trigger depending on state sequences.
	switch state {
	case "ON":
		rt.OffCounter = 0
		if rules.On.Enabled {
			if state == "ON" {
				rt.OnCounter++
			} else {
				rt.OnCounter = 0
			}
		} else {
			rt.OnCounter = 0
		}

		if rules.On.Enabled && rules.On.Threshold > 0 && rt.OnCounter >= rules.On.Threshold {
			// trigger ON
			rt.OnCounter = 0
			rt.OffCounter = 0
			rt.WaitingReverse = true
			rt.LastTriggered = "ON"
			rt.BaseHeight = height

			s := Signal{
				Type:       "ON",
				Height:     height,
				BaseHeight: height,
				State:      "ON",
				TimeISO:    t.UTC().Format(time.RFC3339Nano),
			}
			out = append(out, s)
			logger.Printf("ON_SIGNAL height=%d", height)

			// arm hit
			armHitLocked(height, rules)
		}

	case "OFF":
		rt.OnCounter = 0
		if rules.Off.Enabled {
			if state == "OFF" {
				rt.OffCounter++
			} else {
				rt.OffCounter = 0
			}
		} else {
			rt.OffCounter = 0
		}

		if rules.Off.Enabled && rules.Off.Threshold > 0 && rt.OffCounter >= rules.Off.Threshold {
			// trigger OFF
			rt.OnCounter = 0
			rt.OffCounter = 0
			rt.WaitingReverse = true
			rt.LastTriggered = "OFF"
			rt.BaseHeight = height

			s := Signal{
				Type:       "OFF",
				Height:     height,
				BaseHeight: height,
				State:      "OFF",
				TimeISO:    t.UTC().Format(time.RFC3339Nano),
			}
			out = append(out, s)
			logger.Printf("OFF_SIGNAL height=%d", height)

			armHitLocked(height, rules)
		}
	}

	return out
}

func armHitLocked(triggerHeight int64, rules Rules) {
	// only when just triggered and hit enabled
	if !rules.Hit.Enabled {
		return
	}
	expect := strings.ToUpper(strings.TrimSpace(rules.Hit.Expect))
	if expect != "ON" && expect != "OFF" {
		expect = "ON"
	}
	offset := rules.Hit.Offset
	if offset < 1 {
		offset = 1
	}

	rt.HitWaiting = true
	rt.HitBase = triggerHeight
	rt.HitOffset = offset
	rt.HitExpect = expect
	rt.HitArmedTime = time.Now()
	logger.Printf("HIT_ARMED base=%d offset=%d expect=%s", triggerHeight, offset, expect)
}

func reverseOf(s string) string {
	if s == "ON" {
		return "OFF"
	}
	return "ON"
}

func clamp(v, lo, hi int) int {
	return int(math.Max(float64(lo), math.Min(float64(hi), float64(v))))
}

func isoOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseISOOrNow(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

// ---------- Minimal WebSocket server (standard library only) ----------

type wsConn struct {
	c   net.Conn
	mu  sync.Mutex
	dead atomic.Bool
}

func (w *wsConn) Close() {
	if w.dead.CompareAndSwap(false, true) {
		_ = w.c.Close()
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Note: WS broadcast is not protected by token/ip in this minimal version.
	// If you need, add guards here.
	if !isLoggedIn(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		_ = conn.Close()
		return
	}
	accept := wsAcceptKey(key)

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := buf.WriteString(resp); err != nil {
		_ = conn.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	c := &wsConn{c: conn}
	wsMu.Lock()
	wsClients[c] = struct{}{}
	wsMu.Unlock()

	logger.Printf("WS_CLIENT_CONNECTED remote=%s", r.RemoteAddr)

	// read loop to keep connection healthy (discard frames)
	go func() {
		defer func() {
			wsMu.Lock()
			delete(wsClients, c)
			wsMu.Unlock()
			c.Close()
			logger.Printf("WS_CLIENT_DISCONNECTED remote=%s", r.RemoteAddr)
		}()
		_ = wsReadLoop(conn)
	}()
}

func wsAcceptKey(clientKey string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.Sum([]byte(clientKey + magic))
	return base64.StdEncoding.EncodeToString(h[:])
}

func wsReadLoop(conn net.Conn) error {
	br := bufio.NewReader(conn)
	for {
		// minimal frame parser (masked client-to-server)
		b1, err := br.ReadByte()
		if err != nil {
			return err
		}
		b2, err := br.ReadByte()
		if err != nil {
			return err
		}
		op := b1 & 0x0f
		mask := (b2 & 0x80) != 0
		payloadLen := int(b2 & 0x7f)

		if payloadLen == 126 {
			x1, _ := br.ReadByte()
			x2, _ := br.ReadByte()
			payloadLen = int(uint16(x1)<<8 | uint16(x2))
		} else if payloadLen == 127 {
			// we don't expect huge frames; read 8 bytes
			var n uint64
			for i := 0; i < 8; i++ {
				b, e := br.ReadByte()
				if e != nil {
					return e
				}
				n = (n << 8) | uint64(b)
			}
			if n > 1<<20 {
				return errors.New("ws frame too large")
			}
			payloadLen = int(n)
		}

		var maskKey [4]byte
		if mask {
			if _, err := io.ReadFull(br, maskKey[:]); err != nil {
				return err
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(br, payload); err != nil {
			return err
		}
		if mask {
			for i := range payload {
				payload[i] ^= maskKey[i%4]
			}
		}

		// handle close
		if op == 0x8 {
			return io.EOF
		}
		// ping/pong ignored (browser handles)
	}
}

func wsWriteText(conn net.Conn, msg []byte) error {
	// server-to-client frames are NOT masked
	// FIN=1, opcode=1 (text)
	var hdr bytes.Buffer
	hdr.WriteByte(0x81)

	n := len(msg)
	switch {
	case n <= 125:
		hdr.WriteByte(byte(n))
	case n <= 65535:
		hdr.WriteByte(126)
		hdr.WriteByte(byte((n >> 8) & 0xff))
		hdr.WriteByte(byte(n & 0xff))
	default:
		hdr.WriteByte(127)
		// 8 bytes
		for i := 7; i >= 0; i-- {
			hdr.WriteByte(byte(uint64(n) >> (uint(i) * 8)))
		}
	}

	if _, err := conn.Write(hdr.Bytes()); err != nil {
		return err
	}
	_, err := conn.Write(msg)
	return err
}

func broadcastSignal(s Signal) {
	b, _ := json.Marshal(s)

	wsMu.Lock()
	defer wsMu.Unlock()
	for c := range wsClients {
		if c.dead.Load() {
			continue
		}
		c.mu.Lock()
		err := wsWriteText(c.c, b)
		c.mu.Unlock()
		if err != nil {
			c.Close()
		}
	}
}

// ---------- main ----------

func resetRuntime() {
	rtMu.Lock()
	defer rtMu.Unlock()

	rt.OnCounter = 0
	rt.OffCounter = 0
	rt.WaitingReverse = true
	rt.HitWaiting = false
	rt.BaseHeight = 0

	rt.LastTriggered = ""
	rt.Ring.reset()

	rt.LastHeight = 0
	rt.LastHash = ""
	rt.LastTime = time.Time{}
	rt.Listening = false
}

func main() {
	if err := ensureDirs(); err != nil {
		panic(err)
	}

	lf, err := rotateLogs()
	if err != nil {
		panic(err)
	}
	defer lf.Close()
	logger = log.New(io.MultiWriter(os.Stdout, lf), "", log.LstdFlags|log.Lmicroseconds)

	// abnormal restart marker
	lockPath := filepath.Join(dataDir, "running.lock")
	if _, err := os.Stat(lockPath); err == nil {
		logger.Println("ABNORMAL_RESTART")
	}
	_ = os.WriteFile(lockPath, []byte(time.Now().Format(time.RFC3339Nano)), 0o644)
	defer os.Remove(lockPath)

	logger.Println("SYSTEM_START")

	loaded, err := loadConfig()
	if err != nil {
		logger.Printf("CONFIG_LOAD_ERROR: %v", err)
		// keep default cfg
	}
	cfgMu.Lock()
	cfg = loaded
	// defaults
	if !isValidJudgeRule(cfg.JudgeRule) {
		cfg.JudgeRule = "LUCKY"
	}
	if cfg.Access.Tokens == nil {
		cfg.Access.Tokens = map[string]uint64{}
	}
	// default rules if zero
	if cfg.Rules.Hit.Offset == 0 {
		cfg.Rules.Hit.Offset = 1
	}
	cfgMu.Unlock()

	// runtime must be fully reset every boot
	resetRuntime()

	mux := http.NewServeMux()

	// auth pages
	mux.HandleFunc("/setup", setupPage)
	mux.HandleFunc("/api/setup", setupSubmit)
	mux.HandleFunc("/login", loginPage)
	mux.HandleFunc("/api/login", loginSubmit)
	mux.HandleFunc("/logout", logout)

	// app
	mux.HandleFunc("/", requireLogin(indexHandler))

	// APIs (require login)
	mux.HandleFunc("/api/status", requireLogin(apiStatus))
	mux.HandleFunc("/api/apikey", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			apiGetAPIKeys(w, r)
		case "POST":
			apiSetAPIKeys(w, r)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/rules", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			apiGetRules(w, r)
		case "POST":
			apiSetRules(w, r)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/judge", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			apiGetJudge(w, r)
		case "POST":
			apiSetJudge(w, r)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))

	// SSE + WS (require login)
	mux.HandleFunc("/sse/status", requireLogin(sseStatus))
	mux.HandleFunc("/ws", requireLogin(wsHandler))

	// static assets (only after login gate)
	mux.Handle("/app.js", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("web", "app.js"))
	}))
	mux.Handle("/style.css", requireLogin(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("web", "style.css"))
	}))

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           withSecurityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Printf("HTTP_LISTEN %s", listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("SERVER_ERROR: %v", err)
	}
}

// ---------- headers ----------

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// basic headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

// ---------- unused but ready helpers ----------

// If later you want to guard some external endpoints with IP/token, wrap with externalGuard(handler)
var _ = externalGuard

// Example graceful shutdown if you want:
var _ = context.Background