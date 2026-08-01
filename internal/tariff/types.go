package tariff

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"speedtest/internal/engine"
)

const (
	StatusMeetsAdvertised  = "meets_advertised"
	StatusWithinNormal     = "within_normal"
	StatusBelowNormal      = "below_normal"
	StatusBelowMinimum     = "below_minimum"
	StatusInsufficientData = "insufficient_data"
)

type Tariff struct {
	ID                 int64      `json:"id"`
	ProfileID          int64      `json:"profile_id"`
	ProfileName        string     `json:"profile_name,omitempty"`
	Provider           string     `json:"provider"`
	Name               string     `json:"name"`
	AccessTechnology   string     `json:"access_technology,omitempty"`
	AdvertisedDownMbps float64    `json:"advertised_down_mbps"`
	AdvertisedUpMbps   float64    `json:"advertised_up_mbps"`
	NormalDownMbps     float64    `json:"normal_down_mbps,omitempty"`
	NormalUpMbps       float64    `json:"normal_up_mbps,omitempty"`
	MinimumDownMbps    float64    `json:"minimum_down_mbps,omitempty"`
	MinimumUpMbps      float64    `json:"minimum_up_mbps,omitempty"`
	ValidFrom          time.Time  `json:"valid_from"`
	ValidTo            *time.Time `json:"valid_to,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type MetricComparison struct {
	ActualMbps     float64 `json:"actual_mbps"`
	AdvertisedMbps float64 `json:"advertised_mbps"`
	NormalMbps     float64 `json:"normal_mbps,omitempty"`
	MinimumMbps    float64 `json:"minimum_mbps,omitempty"`
	Percent        float64 `json:"percent"`
	DeviationMbps  float64 `json:"deviation_mbps"`
	Status         string  `json:"status"`
}

type Comparison struct {
	ResultID int64            `json:"result_id"`
	Tariff   Tariff           `json:"tariff"`
	Download MetricComparison `json:"download"`
	Upload   MetricComparison `json:"upload"`
}

func Validate(t Tariff) error {
	if t.ProfileID <= 0 {
		return errors.New("profile_id is required")
	}
	if strings.TrimSpace(t.Provider) == "" {
		return errors.New("provider is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tariff name is required")
	}
	if t.AdvertisedDownMbps <= 0 || t.AdvertisedUpMbps <= 0 {
		return errors.New("advertised download and upload must be greater than zero")
	}
	if err := validateThresholds("download", t.MinimumDownMbps, t.NormalDownMbps, t.AdvertisedDownMbps); err != nil {
		return err
	}
	return validateThresholds("upload", t.MinimumUpMbps, t.NormalUpMbps, t.AdvertisedUpMbps)
}

func validateThresholds(name string, minimum, normal, advertised float64) error {
	if minimum < 0 || normal < 0 {
		return fmt.Errorf("%s thresholds cannot be negative", name)
	}
	if normal > 0 && normal > advertised {
		return fmt.Errorf("normal %s cannot exceed advertised %s", name, name)
	}
	if minimum > 0 && minimum > advertised {
		return fmt.Errorf("minimum %s cannot exceed advertised %s", name, name)
	}
	if minimum > 0 && normal > 0 && minimum > normal {
		return fmt.Errorf("minimum %s cannot exceed normal %s", name, name)
	}
	return nil
}

func Compare(result engine.TestResult, plan Tariff) Comparison {
	return Comparison{
		ResultID: result.ID,
		Tariff:   plan,
		Download: compareMetric(result.DownloadMbps, plan.AdvertisedDownMbps, plan.NormalDownMbps, plan.MinimumDownMbps),
		Upload:   compareMetric(result.UploadMbps, plan.AdvertisedUpMbps, plan.NormalUpMbps, plan.MinimumUpMbps),
	}
}

func compareMetric(actual, advertised, normal, minimum float64) MetricComparison {
	comparison := MetricComparison{
		ActualMbps: actual, AdvertisedMbps: advertised,
		NormalMbps: normal, MinimumMbps: minimum,
		Status: StatusInsufficientData,
	}
	if actual <= 0 || advertised <= 0 {
		return comparison
	}
	comparison.Percent = math.Round(actual/advertised*10000) / 100
	comparison.DeviationMbps = math.Round((actual-advertised)*100) / 100
	switch {
	case actual >= advertised:
		comparison.Status = StatusMeetsAdvertised
	case minimum > 0 && actual < minimum:
		comparison.Status = StatusBelowMinimum
	case normal > 0 && actual >= normal:
		comparison.Status = StatusWithinNormal
	default:
		comparison.Status = StatusBelowNormal
	}
	return comparison
}

// FromSnapshot returns the persisted comparison values of a result. Older
// rows without snapshots are calculated on demand for backwards compatibility.
func FromSnapshot(result engine.TestResult, plan Tariff) Comparison {
	if result.TariffDownStatus == "" && result.TariffUpStatus == "" {
		return Compare(result, plan)
	}
	return Comparison{
		ResultID: result.ID,
		Tariff:   plan,
		Download: MetricComparison{
			ActualMbps: result.DownloadMbps, AdvertisedMbps: plan.AdvertisedDownMbps,
			NormalMbps: plan.NormalDownMbps, MinimumMbps: plan.MinimumDownMbps,
			Percent: result.TariffDownPercent, DeviationMbps: result.TariffDownDeviationMbps,
			Status: result.TariffDownStatus,
		},
		Upload: MetricComparison{
			ActualMbps: result.UploadMbps, AdvertisedMbps: plan.AdvertisedUpMbps,
			NormalMbps: plan.NormalUpMbps, MinimumMbps: plan.MinimumUpMbps,
			Percent: result.TariffUpPercent, DeviationMbps: result.TariffUpDeviationMbps,
			Status: result.TariffUpStatus,
		},
	}
}
