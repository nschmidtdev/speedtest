package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"speedtest/internal/engine"
	"speedtest/internal/storage"
)

func newTariffTestState(t *testing.T) *AppState {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "api-tariff.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	return &AppState{DB: db}
}

func TestTariffCatalogHandlerReturnsProviderSpecificTemplates(t *testing.T) {
	state := newTariffTestState(t)
	rr := httptest.NewRecorder()
	state.TariffCatalogHandler(rr, httptest.NewRequest(http.MethodGet, "/api/tariff-catalog", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var response struct {
		VerifiedAt string `json:"verified_at"`
		Providers  []struct {
			ID      string `json:"id"`
			Tariffs []any  `json:"tariffs"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.VerifiedAt == "" || len(response.Providers) < 5 {
		t.Fatalf("unexpected catalog response: %#v", response)
	}
}

func TestTariffsHandlerCreatesAndListsActiveTariff(t *testing.T) {
	state := newTariffTestState(t)
	body := `{"profile_id":1,"provider":"Vodafone","name":"CableMax 1000","access_technology":"Kabel","advertised_down_mbps":1000,"advertised_up_mbps":50,"normal_down_mbps":850,"normal_up_mbps":45,"minimum_down_mbps":600,"minimum_up_mbps":35,"valid_from":"2026-08-01T00:00:00Z"}`

	create := httptest.NewRecorder()
	state.TariffsHandler(create, httptest.NewRequest(http.MethodPost, "/api/tariffs", strings.NewReader(body)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}

	list := httptest.NewRecorder()
	state.TariffsHandler(list, httptest.NewRequest(http.MethodGet, "/api/tariffs", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
	var tariffs []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &tariffs); err != nil {
		t.Fatal(err)
	}
	if len(tariffs) != 1 || tariffs[0]["provider"] != "Vodafone" {
		t.Fatalf("unexpected tariffs: %#v", tariffs)
	}
}

func TestTariffsHandlerRejectsInvalidThresholds(t *testing.T) {
	state := newTariffTestState(t)
	body := `{"profile_id":1,"provider":"X","name":"Bad","advertised_down_mbps":100,"advertised_up_mbps":20,"normal_down_mbps":120}`
	rr := httptest.NewRecorder()
	state.TariffsHandler(rr, httptest.NewRequest(http.MethodPost, "/api/tariffs", strings.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

func TestTariffComparisonHandlerUsesResultSnapshot(t *testing.T) {
	state := newTariffTestState(t)
	body := `{"profile_id":1,"provider":"ExampleNet","name":"Fiber 500","advertised_down_mbps":500,"advertised_up_mbps":100,"valid_from":"2026-08-01T00:00:00Z"}`
	create := httptest.NewRecorder()
	state.TariffsHandler(create, httptest.NewRequest(http.MethodPost, "/api/tariffs", strings.NewReader(body)))

	resultID, err := storage.InsertResult(state.DB, engine.TestResult{
		ProfileID: 1, MeasuredAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		DownloadMbps: 400, UploadMbps: 90, Status: "success",
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tariff-comparison?result_id="+strconv.FormatInt(resultID, 10), nil)
	state.TariffComparisonHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("comparison status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var comparison struct {
		Download struct {
			Percent       float64 `json:"percent"`
			DeviationMbps float64 `json:"deviation_mbps"`
		} `json:"download"`
		Upload struct {
			Percent       float64 `json:"percent"`
			DeviationMbps float64 `json:"deviation_mbps"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.Download.Percent != 80 || comparison.Upload.Percent != 90 {
		t.Fatalf("unexpected percentages: %#v", comparison)
	}
	if comparison.Download.DeviationMbps != -100 || comparison.Upload.DeviationMbps != -10 {
		t.Fatalf("unexpected deviations: %#v", comparison)
	}
}
