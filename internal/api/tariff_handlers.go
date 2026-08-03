package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"speedtest/internal/storage"
	"speedtest/internal/tariff"
)

// TariffCatalogHandler — GET /api/tariff-catalog.
func (s *AppState) TariffCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, tariff.Catalog())
}

// TariffsHandler — GET/POST /api/tariffs.
// GET: JSON für JS, HTML für HTMX. POST: Form-Decode + JSON-Decode (Dual-Mode).
func (s *AppState) TariffsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		plans, err := storage.ListActiveTariffs(s.DB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if plans == nil {
			plans = []tariff.Tariff{}
		}
		if isHTMX(r) {
			renderPartial(w, "tariff_list.html", plans)
			return
		}
		writeJSON(w, plans)

	case http.MethodPost:
		var plan tariff.Tariff
		if isHTMX(r) {
			// HTMX schickt Form-Data
			if err := r.ParseForm(); err != nil {
				renderHTMXError(w, "invalid form: "+err.Error())
				return
			}
			plan = tariffFromForm(r)
		} else {
			// JS schickt JSON
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&plan); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
				return
			}
		}
		if err := tariff.Validate(plan); err != nil {
			if isHTMX(r) {
				renderHTMXError(w, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		id, err := storage.CreateTariffVersion(s.DB, plan)
		if err != nil {
			if isHTMX(r) {
				renderHTMXError(w, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := storage.GetTariffByID(s.DB, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if isHTMX(r) {
			// Nach Erfolg: aktualisierte HTML-Liste zurückgeben
			plans, err := storage.ListActiveTariffs(s.DB)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if plans == nil {
				plans = []tariff.Tariff{}
			}
			renderPartial(w, "tariff_list.html", plans)
			return
		}
		writeJSON(w, created)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// tariffFromForm parst die relevanten Felder aus einem multipart/form-encoded Request.
func tariffFromForm(r *http.Request) tariff.Tariff {
	parseFloat := func(key string) float64 {
		v, _ := strconv.ParseFloat(r.FormValue(key), 64)
		return v
	}
	parseInt64 := func(key string) int64 {
		v, _ := strconv.ParseInt(r.FormValue(key), 10, 64)
		return v
	}

	// Provider: entweder aus Dropdown oder Custom-Eingabe
	provider := r.FormValue("provider")
	if provider == "" {
		provider = r.FormValue("provider_select")
	}
	// Tarifname: entweder aus Template-Dropdown oder Custom-Eingabe
	name := r.FormValue("name")
	if name == "" {
		name = r.FormValue("tariff_template")
	}

	return tariff.Tariff{
		ProfileID:           parseInt64("profile_id"),
		Provider:            provider,
		Name:                name,
		AccessTechnology:    r.FormValue("access_technology"),
		AdvertisedDownMbps:  parseFloat("advertised_down_mbps"),
		AdvertisedUpMbps:    parseFloat("advertised_up_mbps"),
		NormalDownMbps:      parseFloat("normal_down_mbps"),
		NormalUpMbps:        parseFloat("normal_up_mbps"),
		MinimumDownMbps:     parseFloat("minimum_down_mbps"),
		MinimumUpMbps:       parseFloat("minimum_up_mbps"),
		ValidFrom:           time.Now().UTC(),
	}
}

// TariffComparisonHandler — GET /api/tariff-comparison?result_id=123.
func (s *AppState) TariffComparisonHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	resultID, err := strconv.ParseInt(r.URL.Query().Get("result_id"), 10, 64)
	if err != nil || resultID <= 0 {
		writeError(w, http.StatusBadRequest, "valid result_id is required")
		return
	}
	result, err := storage.GetResultByID(s.DB, resultID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "result not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.TariffID == 0 {
		writeError(w, http.StatusNotFound, "no tariff assigned to this result")
		return
	}
	plan, err := storage.GetTariffByID(s.DB, result.TariffID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "tariff snapshot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, tariff.FromSnapshot(*result, *plan))
}

// TariffDeleteHandler — DELETE /api/tariffs/{id}
// Löscht den Tarif und gibt leeren Body zurück (HTMX entfernt die Karte per outerHTML).
func (s *AppState) TariffDeleteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/tariffs/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid tariff ID", http.StatusBadRequest)
		return
	}
	if err := storage.DeleteTariff(s.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TariffDetailHandler — GET /api/tariffs/{id}
// Gibt einen einzelnen Tarif als JSON zurück (für JS-Edit-Prefill).
func (s *AppState) TariffDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/tariffs/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid tariff ID")
		return
	}
	plan, err := storage.GetTariffByID(s.DB, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "tariff not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, plan)
}
