package httpapi

import (
	"net/http"
	"strings"

	"tron-signal/backend/app"
	"tron-signal/backend/auth"
	"tron-signal/backend/config"
)

func loginHandler(core *app.Core, store *config.Store, sessions *auth.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)

		// 如果还没设置账号密码：跳转到首次设置页（由前端页面处理）
		cfg := store.Get()
		if cfg.AdminUser == "" || cfg.AdminPassHash == "" {
			http.Redirect(w, r, "/#setup", http.StatusFound)
			return
		}

		if r.Method == http.MethodGet {
			http.ServeFile(w, r, "web/index.html")
			return
		}

		if r.Method != http.MethodPost {
			JSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"ok":    false,
				"error": "METHOD_NOT_ALLOWED",
			})
			return
		}

		_ = r.ParseForm()
		u := strings.TrimSpace(r.FormValue("user"))
		p := r.FormValue("pass")

		if !store.VerifyAdmin(u, p) {
			JSONStatus(w, http.StatusUnauthorized, map[string]any{
				"ok":    false,
				"error": "BAD_CREDENTIALS",
			})
			return
		}

		sessions.Login(w)
		JSON(w, map[string]any{"ok": true})
	}
}

func logoutHandler(store *config.Store, sessions *auth.MemoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		sessions.Logout(w, r)
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func adminPasswordHandler(core *app.Core, store *config.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		if r.Method != http.MethodPost {
			JSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"ok":    false,
				"error": "METHOD_NOT_ALLOWED",
			})
			return
		}

		_ = r.ParseForm()
		u := strings.TrimSpace(r.FormValue("user"))
		p := r.FormValue("pass")
		if u == "" || p == "" {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "MISSING_USER_OR_PASS",
			})
			return
		}

		if err := store.SetAdmin(u, p); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		JSON(w, map[string]any{"ok": true})
	}
}
