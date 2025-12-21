package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func JSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func NoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func NowISO8601InUTC8() string {
	// UI 强制北京时间（UTC+8）
	loc := time.FixedZone("UTC+8", 8*3600)
	return time.Now().In(loc).Format(time.RFC3339)
}
