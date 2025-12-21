package httpapi

import (
	"net/http"
	"strings"

	"tron-signal/backend/app"
)

func apiJudgeSwitchHandler(core *app.Core) http.HandlerFunc {
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
		rule := strings.TrimSpace(r.FormValue("rule"))
		confirm := strings.TrimSpace(r.FormValue("confirm"))

		if rule == "" {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "MISSING_RULE",
			})
			return
		}

		// second confirmation: confirm must be "yes"
		if confirm != "yes" {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "NEED_CONFIRM",
			})
			return
		}

		if err := core.SwitchJudgeRule(rule); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		JSON(w, map[string]any{
			"ok":   true,
			"rule": rule,
		})
	}
}
