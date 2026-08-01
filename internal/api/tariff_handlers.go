package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
		writeJSON(w, plans)
	case http.MethodPost:
		var plan tariff.Tariff
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&plan); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := tariff.Validate(plan); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		id, err := storage.CreateTariffVersion(s.DB, plan)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := storage.GetTariffByID(s.DB, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, created)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
