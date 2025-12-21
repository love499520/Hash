package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthStore：内存态（Token 列表 + 简单登录态 session）
// Token/白名单持久化在 config（由上层注入/落盘）
type AuthStore struct {
	mu sync.RWMutex

	// session: sessionID -> expire
	sessions map[string]time.Time
}

func NewAuthStore() *AuthStore {
	return &AuthStore{
		sessions: map[string]time.Time{},
	}
}

func NewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *AuthStore) CreateSession(ttl time.Duration) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	sid := NewSessionID()
	a.sessions[sid] = time.Now().Add(ttl)
	return sid
}

func (a *AuthStore) ValidSession(sessionID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	exp, ok := a.sessions[sessionID]
	if !ok {
		return false
	}
	return time.Now().Before(exp)
}

func (a *AuthStore) DeleteSession(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
}

// ClientIP：提取客户端 IP（优先 X-Forwarded-For / X-Real-IP）
func ClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	xrip := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// InWhitelist：白名单支持
// - 单 IP： "1.2.3.4"
// - CIDR： "1.2.3.0/24"
func InWhitelist(ip string, whitelist []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, w := range whitelist {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if strings.Contains(w, "/") {
			_, cidr, err := net.ParseCIDR(w)
			if err == nil && cidr.Contains(parsed) {
				return true
			}
			continue
		}
		if net.ParseIP(w) != nil && w == ip {
			return true
		}
	}
	return false
}

// ExtractToken：从 Header / Query 中取 token
func ExtractToken(r *http.Request) string {
	// 推荐：Header
	if t := strings.TrimSpace(r.Header.Get("X-Token")); t != "" {
		return t
	}
	// 兼容：query
	if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
		return t
	}
	return ""
}
