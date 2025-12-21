package httpapi

import (
	"encoding/json"
	"net/http"

	"tron-signal/backend/app"
)

func apiSourcesListHandler(core *app.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		if r.Method != http.MethodGet {
			JSONStatus(w, http.StatusMethodNotAllowed, map[string]any{
				"ok":    false,
				"error": "METHOD_NOT_ALLOWED",
			})
			return
		}

		JSON(w, map[string]any{
			"ok":      true,
			"sources": core.GetSources(),
		})
	}
}

func apiSourcesUpsertHandler(core *app.Core) http.HandlerFunc {
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

		if err := core.UpsertSource(body); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		JSON(w, map[string]any{"ok": true})
	}
}

func apiSourcesDeleteHandler(core *app.Core) http.HandlerFunc {
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
		id := r.FormValue("id")
		if id == "" {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "MISSING_ID",
			})
			return
		}

		if err := core.DeleteSource(id); err != nil {
			JSONStatus(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		JSON(w, map[string]any{"ok": true})
	}
}
