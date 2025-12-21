package httpapi

import (
	"encoding/json"
	"net/http"

	"tron-signal/backend/app"
)

type PublicHandlers struct {
	Core *app.Core
}

// GET /api/status
func (h *PublicHandlers) Status(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": h.Core.Status(),
	})
}

// GET /api/blocks
func (h *PublicHandlers) Blocks(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": h.Core.Blocks(),
	})
}
