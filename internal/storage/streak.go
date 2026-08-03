package storage

import (
	"database/sql"
	"time"

	"speedtest/internal/engine"
)

// StreakDay ist die pro-Werktag aggregierte Bewertung.
type StreakDay struct {
	Date      time.Time `json:"date"`
	Workday   bool      `json:"workday"`
	DownAvg   float64   `json:"down_avg"`
	UpAvg     float64   `json:"up_avg"`
	DownBelow bool      `json:"down_below"`
	UpBelow   bool      `json:"up_below"`
	Below     bool      `json:"below"` // Down ODER Up unter Normalwert
}

// StreakResult fasst die Werktag-Strähne zusammen.
type StreakResult struct {
	CurrentStreak   int         `json:"current_streak"`
	TotalBelowDays  int         `json:"total_below_days"`
	WindowDays      int         `json:"window_days"`
	Days            []StreakDay `json:"days"`
	ReadyToComplain bool        `json:"ready_to_complain"` // currentStreak >= 2
}

// ComputeStreak berechnet die konsekutive Werktag-Strähne der
// Tarif-Unterschreitung für ein Profil. Ein Werktag (Mo–Sa) gilt als
// "below", wenn das Tagesmittel von Down- ODER Up-Messung unter dem
// Normalwert liegt. Wenn der Tarif keinen Normalwert setzt (0), greift
// der Fallback 90 % des advertised-Werts (BNetzA-Praxis).
func ComputeStreak(db *sql.DB, profileID int64, normalDown, normalUp,
	advertisedDown, advertisedUp float64, windowDays int) StreakResult {

	if windowDays < 1 {
		windowDays = 30
	}

	// Ergebnisse laden (mit Puffer vor Fensterbeginn)
	cutoff := time.Now().AddDate(0, 0, -windowDays-2)
	results, _ := GetResults(db, profileID, 5000, cutoff, time.Now())

	// Nach Werktag gruppieren
	byDay := map[string][]engine.TestResult{}
	for _, r := range results {
		if r.Status != "success" {
			continue
		}
		key := r.MeasuredAt.Local().Format("2006-01-02")
		byDay[key] = append(byDay[key], r)
	}

	// Fallback-Normalwerte
	if normalDown == 0 {
		normalDown = advertisedDown * 0.9
	}
	if normalUp == 0 {
		normalUp = advertisedUp * 0.9
	}

	// Tage im Fenster aufbauen (ältester zuerst)
	now := time.Now()
	startDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -(windowDays-1))
	days := make([]StreakDay, 0, windowDays)
	for i := 0; i < windowDays; i++ {
		date := startDay.AddDate(0, 0, i)
		d := StreakDay{Date: date, Workday: date.Weekday() != time.Sunday}
		if measurements, ok := byDay[date.Format("2006-01-02")]; ok {
			var downSum, upSum float64
			var n int
			for _, r := range measurements {
				if r.DownloadMbps > 0 {
					downSum += r.DownloadMbps
					n++
				}
				upSum += r.UploadMbps
			}
			if n > 0 {
				d.DownAvg = downSum / float64(n)
				d.UpAvg = upSum / float64(n)
				d.DownBelow = d.DownAvg < normalDown
				d.UpBelow = d.UpAvg < normalUp
				d.Below = d.DownBelow || d.UpBelow
			}
		}
		days = append(days, d)
	}

	// Current Streak: vom letzten Tag rückwärts zählen, nur Werktage.
	// Ein Tag ohne Messungen bricht die Strähne nicht (Lücke), zählt aber nicht.
	streak := 0
	for i := len(days) - 1; i >= 0; i-- {
		d := days[i]
		if !d.Workday {
			continue // Sonntag überspringen
		}
		if d.Below {
			streak++
		} else if d.DownAvg > 0 || d.UpAvg > 0 {
			break // Tag hatte Messungen, war aber ok → Strähne bricht
		}
	}

	totalBelow := 0
	for _, d := range days {
		if d.Below {
			totalBelow++
		}
	}

	return StreakResult{
		CurrentStreak:   streak,
		TotalBelowDays:  totalBelow,
		WindowDays:      windowDays,
		Days:            days,
		ReadyToComplain: streak >= 2,
	}
}
