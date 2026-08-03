package api

import (
	"net/http"

	"speedtest/internal/storage"
	"speedtest/internal/tariff"
)

// TariffStreakHandler — GET /api/tariff-streak?profile_id=1&days=30
// Liefert die konsekutive Werktag-Strähne der Tarif-Unterschreitung.
func (s *AppState) TariffStreakHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	profileID := int64(parseQueryInt(r, "profile_id", 0))
	if profileID <= 0 {
		writeError(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	windowDays := parseQueryInt(r, "days", 30)
	if windowDays < 7 || windowDays > 90 {
		windowDays = 30
	}

	// Aktiven Tarif für das Profil finden
	plans, err := storage.ListActiveTariffs(s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var plan *tariff.Tariff
	for i := range plans {
		if plans[i].ProfileID == profileID {
			plan = &plans[i]
			break
		}
	}
	if plan == nil {
		writeError(w, http.StatusNotFound, "kein aktiver Tarif für dieses Profil")
		return
	}

	streak := storage.ComputeStreak(s.DB, profileID,
		plan.NormalDownMbps, plan.NormalUpMbps,
		plan.AdvertisedDownMbps, plan.AdvertisedUpMbps, windowDays)

	writeJSON(w, map[string]any{
		"profile_id":        profileID,
		"tariff":            plan,
		"current_streak":    streak.CurrentStreak,
		"total_below_days":  streak.TotalBelowDays,
		"window_days":       streak.WindowDays,
		"ready_to_complain": streak.ReadyToComplain,
		"days":              streak.Days,
	})
}
