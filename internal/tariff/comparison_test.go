package tariff

import (
	"testing"

	"speedtest/internal/engine"
)

func TestValidateAcceptsConsistentTariff(t *testing.T) {
	tariff := Tariff{
		ProfileID:          1,
		Provider:           "Vodafone",
		Name:               "CableMax 1000",
		AdvertisedDownMbps: 1000,
		AdvertisedUpMbps:   50,
		NormalDownMbps:     850,
		NormalUpMbps:       45,
		MinimumDownMbps:    600,
		MinimumUpMbps:      35,
	}

	if err := Validate(tariff); err != nil {
		t.Fatalf("expected valid tariff, got %v", err)
	}
}

func TestValidateRejectsInconsistentThresholdOrder(t *testing.T) {
	tariff := Tariff{
		ProfileID:          1,
		Provider:           "ExampleNet",
		Name:               "Fiber 500",
		AdvertisedDownMbps: 500,
		AdvertisedUpMbps:   100,
		NormalDownMbps:     550,
		MinimumDownMbps:    300,
	}

	if err := Validate(tariff); err == nil {
		t.Fatal("expected normal download above advertised download to be rejected")
	}
}

func TestCompareCalculatesDownloadAndUploadAgainstTariff(t *testing.T) {
	plan := Tariff{
		ID:                 7,
		ProfileID:          1,
		Provider:           "Vodafone",
		Name:               "CableMax 1000",
		AdvertisedDownMbps: 1000,
		AdvertisedUpMbps:   50,
		NormalDownMbps:     850,
		NormalUpMbps:       45,
		MinimumDownMbps:    600,
		MinimumUpMbps:      35,
	}
	result := engine.TestResult{ID: 11, DownloadMbps: 755.6, UploadMbps: 42.6}

	comparison := Compare(result, plan)

	if comparison.Download.Percent != 75.56 {
		t.Fatalf("download percent = %.2f, want 75.56", comparison.Download.Percent)
	}
	if comparison.Upload.Percent != 85.2 {
		t.Fatalf("upload percent = %.2f, want 85.20", comparison.Upload.Percent)
	}
	if comparison.Download.DeviationMbps != -244.4 {
		t.Fatalf("download deviation = %.2f, want -244.40", comparison.Download.DeviationMbps)
	}
	if comparison.Upload.DeviationMbps != -7.4 {
		t.Fatalf("upload deviation = %.2f, want -7.40", comparison.Upload.DeviationMbps)
	}
	if comparison.Download.Status != StatusBelowNormal {
		t.Fatalf("download status = %q, want %q", comparison.Download.Status, StatusBelowNormal)
	}
	if comparison.Upload.Status != StatusBelowNormal {
		t.Fatalf("upload status = %q, want %q", comparison.Upload.Status, StatusBelowNormal)
	}
}

func TestCompareMarksMissingMeasurementAsInsufficientData(t *testing.T) {
	plan := Tariff{AdvertisedDownMbps: 100, AdvertisedUpMbps: 40}
	comparison := Compare(engine.TestResult{}, plan)

	if comparison.Download.Status != StatusInsufficientData || comparison.Upload.Status != StatusInsufficientData {
		t.Fatalf("zero measurements must be insufficient data: %#v", comparison)
	}
}
