package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"tron-signal/backend/auth"
	"tron-signal/backend/judge"
	"tron-signal/backend/machine"
	"tron-signal/backend/source"
)

// Config
// - data/config.json 持久化结构
// - 同时实现 auth.ConfigReader（白名单/Token/管理员账号校验）
//
// 约束：
// - 鉴权模型只区分【内网白名单 / 外网Token】
// - 不区分 HTTP/WS（路由入口统一套门禁）
// - 管理登录态由 cookie session（auth.AuthStore）维护
type Config struct {
	mu sync.RWMutex `json:"-"`

	// ====== 管理账号（用于 /api/admin/login） ======
	Admin struct {
		Username     string `json:"username"`
		PasswordSalt string `json:"password_salt"` // hex
		PasswordHash string `json:"password_hash"` // sha256(salt + password) hex
	} `json:"admin"`

	// ====== 外网 Token（用于门禁） ======
	Tokens []string `json:"tokens"`

	// ====== 内网 IP 白名单（免 Token） ======
	Whitelist []string `json:"whitelist"`

	// ====== 判定规则（Lucky/Big/Odd） ======
	JudgeRule judge.RuleType `json:"judge_rule"`

	// ====== 状态机列表（多实例） ======
	Machines []machine.Config `json:"machines"`

	// ====== 数据源列表（多源，先到先用） ======
	Sources []source.Config `json:"sources"`

	// ====== 源扩展配置（如 Ankr RPC 的 method/params） ======
	SourceExtras map[string]SourceExtra `json:"source_extras"`

	// ====== 轮询策略（Runner） ======
	Poll struct {
		BaseTickMS      int  `json:"base_tick_ms"`       // 基础节拍（毫秒）
		AutoRestart     bool `json:"auto_restart"`       // 失败自动等待后重试
		FailWaitMinutes int  `json:"fail_wait_minutes"` // N 分钟
	} `json:"poll"`

	// ====== 内部：配置文件路径 ======
	path string `json:"-"`
}

type SourceExtra struct {
	// Ankr RPC：第 1 次请求的 method/params（用于拿最新高度或直接拿 block）
	RPCMethod string `json:"rpc_method"`
	RPCParams any    `json:"rpc_params"`

	// Ankr RPC：第 2 次请求的 block method（拿 hash/timestamp）
	// 对应 ankr_rpc_fetcher.go 里的 cfg.Headers["X-RPC-BLOCK-METHOD"]
	RPCBlockMethod string `json:"rpc_block_method"`
}

func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		log.Printf("CONFIG_LOAD_FAIL: %v\n", err)
		cfg = Default()
		cfg.path = path
		_ = cfg.Save()
		return cfg
	}
	cfg.path = path
	return cfg
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	// 兼容：空 extras
	if cfg.SourceExtras == nil {
		cfg.SourceExtras = map[string]SourceExtra{}
	}
	cfg.path = path

	// 默认值补齐
	if cfg.Poll.BaseTickMS <= 0 {
		cfg.Poll.BaseTickMS = 800
	}
	if cfg.Poll.FailWaitMinutes <= 0 {
		cfg.Poll.FailWaitMinutes = 2
	}
	// 默认规则
	if cfg.JudgeRule == "" {
		cfg.JudgeRule = judge.Lucky
	}

	return &cfg, nil
}

func Default() *Config {
	cfg := &Config{
		Tokens:       []string{},
		Whitelist:    []string{},
		JudgeRule:    judge.Lucky,
		Machines:     []machine.Config{},
		Sources:      []source.Config{},
		SourceExtras: map[string]SourceExtra{},
	}
	cfg.Poll.BaseTickMS = 800
	cfg.Poll.AutoRestart = true
	cfg.Poll.FailWaitMinutes = 2
	return cfg
}

func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.path == "" {
		return nil
	}
	tmp := c.path + ".tmp"
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// ====== auth.ConfigReader 实现（门禁层使用） ======

func (c *Config) GetWhitelist() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]string, 0, len(c.Whitelist))
	out = append(out, c.Whitelist...)
	return out
}

func (c *Config) HasToken(token string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, t := range c.Tokens {
		if t == token {
			return true
		}
	}
	return false
}

func (c *Config) CheckAdmin(username, password string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Admin.Username == "" || c.Admin.PasswordSalt == "" || c.Admin.PasswordHash == "" {
		return false
	}
	if username != c.Admin.Username {
		return false
	}
	return hashPasswordHex(c.Admin.PasswordSalt, password) == c.Admin.PasswordHash
}

// SetAdmin：你管理页设置密码时用（后续会有 /api/admin/set_password）
// 这里先提供函数，路由 handler 下一步会接上。
func (c *Config) SetAdmin(username, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Admin.Username = username
	salt := auth.NewToken() // 32 hex
	c.Admin.PasswordSalt = salt
	c.Admin.PasswordHash = hashPasswordHex(salt, password)
	_ = c.Save()
}

// ====== Token / 白名单管理（供 handler 调用） ======

func (c *Config) AddToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Tokens = append(c.Tokens, token)
	_ = c.Save()
}

func (c *Config) DeleteToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.Tokens))
	for _, t := range c.Tokens {
		if t != token {
			out = append(out, t)
		}
	}
	c.Tokens = out
	_ = c.Save()
}

func (c *Config) SetWhitelist(list []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Whitelist = list
	_ = c.Save()
}

// ====== 规则切换（判定规则切换要停机清计数器：由上层 Core 调用 StopAllMachinesAndReset） ======

func (c *Config) SetJudgeRule(rule judge.RuleType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.JudgeRule = rule
	_ = c.Save()
}

// ====== 数据源管理 ======

func (c *Config) UpsertSource(sc source.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sources {
		if c.Sources[i].ID == sc.ID {
			c.Sources[i] = sc
			_ = c.Save()
			return
		}
	}
	c.Sources = append(c.Sources, sc)
	_ = c.Save()
}

func (c *Config) DeleteSource(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]source.Config, 0, len(c.Sources))
	for _, s := range c.Sources {
		if s.ID != id {
			out = append(out, s)
		}
	}
	c.Sources = out
	delete(c.SourceExtras, id)
	_ = c.Save()
}

func (c *Config) SetSourceExtra(id string, ex SourceExtra) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.SourceExtras == nil {
		c.SourceExtras = map[string]SourceExtra{}
	}
	c.SourceExtras[id] = ex
	_ = c.Save()
}

// ====== 状态机管理 ======

func (c *Config) SetMachines(ms []machine.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Machines = ms
	_ = c.Save()
}

func (c *Config) SetPoll(baseTickMS int, auto bool, waitMinutes int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if baseTickMS < 200 {
		baseTickMS = 200
	}
	if waitMinutes <= 0 {
		waitMinutes = 1
	}
	c.Poll.BaseTickMS = baseTickMS
	c.Poll.AutoRestart = auto
	c.Poll.FailWaitMinutes = waitMinutes
	_ = c.Save()
}

// ====== helpers ======

func hashPasswordHex(saltHex string, password string) string {
	// sha256( saltHex + ":" + password )
	h := sha256.Sum256([]byte(saltHex + ":" + password))
	return hex.EncodeToString(h[:])
}

// For UI：北京时间 ISO（你清单里要求 UI 用 YYYY/MM/DD HH:mm:ss，前端会格式化）
// 这里仅提供后端时间转换示例（如果你要 /api/status 直接输出北京时间字符串可用）
func toBJ(t time.Time) time.Time {
	// 固定 UTC+8
	return t.In(time.FixedZone("UTC+8", 8*3600))
}
