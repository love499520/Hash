package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

func JSON(w http.ResponseWriter, v any) {
	JSONStatus(w, http.StatusOK, v)
}

func JSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func NoCache(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}

func ClientIP(r *http.Request) string {
	// X-Forwarded-For
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	// X-Real-IP
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); xrip != "" {
		return xrip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func NowBeijing() time.Time {
	return time.Now().In(time.FixedZone("UTC+8", 8*3600))
}
