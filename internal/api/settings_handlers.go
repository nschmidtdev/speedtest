package api

import (
	"encoding/json"
	"net/http"

	"speedtest/internal/storage"
)

// SettingsHandler — GET liefert alle Settings, PUT speichert.
//
//	GET  /api/settings              → { "complaint_name": "...", ... }
//	PUT  /api/settings              → Body: { "complaint_name": "...", ... }
func (s *AppState) SettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m, err := storage.GetAllSettings(s.DB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, m)

	case http.MethodPut:
		var in map[string]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		// Whitelist-Filter: nur erlaubte Keys werden übernommen.
		filtered := make(map[string]string, len(in))
		for k, v := range in {
			if storage.IsAllowedSettingKey(k) {
				filtered[k] = v
			}
		}
		if len(filtered) == 0 {
			writeError(w, http.StatusBadRequest, "no valid settings keys")
			return
		}
		if err := storage.SetSettings(s.DB, filtered); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, filtered)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
