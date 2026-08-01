package storage

import (
	"path/filepath"
	"testing"
	"time"

	"speedtest/internal/engine"
	"speedtest/internal/tariff"
)

func TestCreateTariffVersionClosesPreviousActiveVersion(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tariff-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}

	first := tariff.Tariff{ProfileID: 1, Provider: "Provider A", Name: "250", AdvertisedDownMbps: 250, AdvertisedUpMbps: 50, ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	firstID, err := CreateTariffVersion(db, first)
	if err != nil {
		t.Fatalf("create first tariff: %v", err)
	}
	second := tariff.Tariff{ProfileID: 1, Provider: "Provider A", Name: "1000", AdvertisedDownMbps: 1000, AdvertisedUpMbps: 50, ValidFrom: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	secondID, err := CreateTariffVersion(db, second)
	if err != nil {
		t.Fatalf("create second tariff: %v", err)
	}

	active, err := GetActiveTariff(db, 1, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("get active tariff: %v", err)
	}
	if active.ID != secondID {
		t.Fatalf("active tariff id = %d, want %d", active.ID, secondID)
	}
	old, err := GetTariffByID(db, firstID)
	if err != nil {
		t.Fatalf("get old tariff: %v", err)
	}
	if old.ValidTo == nil || !old.ValidTo.Equal(second.ValidFrom) {
		t.Fatalf("old tariff valid_to = %v, want %v", old.ValidTo, second.ValidFrom)
	}
}

func TestInsertResultSnapshotsActiveTariff(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tariff-result.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}

	measuredAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	tariffID, err := CreateTariffVersion(db, tariff.Tariff{
		ProfileID: 1, Provider: "Provider A", Name: "Fiber 500",
		AdvertisedDownMbps: 500, AdvertisedUpMbps: 100,
		ValidFrom: measuredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = InsertResult(db, engine.TestResult{
		ProfileID: 1, MeasuredAt: measuredAt, DownloadMbps: 400, UploadMbps: 90, Status: "success",
	})
	if err != nil {
		t.Fatalf("insert result: %v", err)
	}

	results, err := GetResults(db, 1, 10, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TariffID != tariffID {
		t.Fatalf("result tariff id = %d, want %d", results[0].TariffID, tariffID)
	}
	result := results[0]
	if result.TariffDownPercent != 80 || result.TariffDownDeviationMbps != -100 || result.TariffDownStatus != tariff.StatusBelowNormal {
		t.Fatalf("download comparison snapshot = %#v", result)
	}
	if result.TariffUpPercent != 90 || result.TariffUpDeviationMbps != -10 || result.TariffUpStatus != tariff.StatusBelowNormal {
		t.Fatalf("upload comparison snapshot = %#v", result)
	}
}

func TestOpenBackfillsMissingComparisonSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tariff-backfill.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	measuredAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	tariffID, err := CreateTariffVersion(db, tariff.Tariff{
		ProfileID: 1, Provider: "Provider A", Name: "Fiber 500",
		AdvertisedDownMbps: 500, AdvertisedUpMbps: 100,
		NormalDownMbps: 450, NormalUpMbps: 90,
		MinimumDownMbps: 300, MinimumUpMbps: 70,
		ValidFrom: measuredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO results
		(profile_id, tariff_id, measured_at, download_mbps, upload_mbps, status)
		VALUES (1, ?, ?, 400, 60, 'success')`, tariffID, measuredAt.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatal(err)
	}
	resultID, _ := res.LastInsertId()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := GetResultByID(db, resultID)
	if err != nil {
		t.Fatal(err)
	}
	if result.TariffDownPercent != 80 || result.TariffDownDeviationMbps != -100 || result.TariffDownStatus != tariff.StatusBelowNormal {
		t.Fatalf("backfilled download snapshot = %#v", result)
	}
	if result.TariffUpPercent != 60 || result.TariffUpDeviationMbps != -40 || result.TariffUpStatus != tariff.StatusBelowMinimum {
		t.Fatalf("backfilled upload snapshot = %#v", result)
	}
}
