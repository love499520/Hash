package auth

import (
	"net/http"
	"strings"

	"tron-signal/internal/config"
)

// AuthMiddleware
// 鉴权模型：
// - 先判断 IP 白名单（内网）→ 直接放行
// - 否则必须携带 Token
func AuthMiddleware(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := strings.Cut(r.RemoteAddr, ":"); err == nil {
			ip = host
		}

		// 内网白名单直接放行
		if InWhitelist(ip, cfg.IPWhitelist) {
			next.ServeHTTP(w, r)
			return
		}

		// 外网必须校验 Token
		token := r.Header.Get("X-Token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		var tokens []string
		for _, t := range cfg.Tokens {
			tokens = append(tokens, t.Value)
		}

		if !HasToken(token, tokens) {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
