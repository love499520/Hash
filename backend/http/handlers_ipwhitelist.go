package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"tron-signal/backend/config"
)

func apiIPWhitelistGetHandler(store *config.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		if r.Method != http.MethodGet {
			JSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"ok":    false,
				"error": "METHOD_NOT_ALLOWED",
			})
			return
		}

		cfg := store.Get()
		JSON(w, map[string]any{
			"ok":         true,
			"ipWhitelist": cfg.IPWhitelist,
		})
	}
}

func apiIPWhitelistSaveHandler(store *config.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		if r.Method != http.MethodPost {
			JSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"ok":    false,
				"error": "METHOD_NOT_ALLOWED",
			})
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "BAD_JSON",
			})
			return
		}

		raw, _ := body["ipWhitelist"].(string)
		raw = strings.TrimSpace(raw)

		list := []string{}
		if raw != "" {
			for _, line := range strings.Split(raw, "\n") {
				v := strings.TrimSpace(line)
				if v == "" {
					continue
				}
				list = append(list, v)
			}
		}

		if err := store.SetIPWhitelist(list); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		JSON(w, map[string]any{"ok": true})
	}
}
