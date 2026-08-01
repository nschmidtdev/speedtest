package api

import (
	"net/http"
	"time"

	"speedtest/internal/storage"
)

type metricSummary struct {
	Average float64   `json:"average"`
	Low     float64   `json:"low"`
	LowAt   time.Time `json:"low_at"`
	High    float64   `json:"high"`
	HighAt  time.Time `json:"high_at"`
	Best    float64   `json:"best"`
	BestAt  time.Time `json:"best_at"`
	Worst   float64   `json:"worst"`
	WorstAt time.Time `json:"worst_at"`
	Count   int       `json:"count"`
}

type dailyStats struct {
	Date         string  `json:"date"`
	Download     float64 `json:"download"`
	Upload       float64 `json:"upload"`
	Ping         float64 `json:"ping"`
	Jitter       float64 `json:"jitter"`
	DownloadLow  float64 `json:"download_low"`
	DownloadHigh float64 `json:"download_high"`
	UploadLow    float64 `json:"upload_low"`
	UploadHigh   float64 `json:"upload_high"`
	PingLow      float64 `json:"ping_low"`
	PingHigh     float64 `json:"ping_high"`
	JitterLow    float64 `json:"jitter_low"`
	JitterHigh   float64 `json:"jitter_high"`
}

type statAccumulator struct {
	sum           float64
	count         int
	low, high     float64
	lowAt, highAt time.Time
}

func (a *statAccumulator) add(v float64, at time.Time) {
	if v <= 0 {
		return
	}
	if a.count == 0 || v < a.low {
		a.low, a.lowAt = v, at
	}
	if a.count == 0 || v > a.high {
		a.high, a.highAt = v, at
	}
	a.sum += v
	a.count++
}

func (a statAccumulator) summary(lowerIsBetter bool) metricSummary {
	if a.count == 0 {
		return metricSummary{}
	}
	s := metricSummary{Average: a.sum / float64(a.count), Low: a.low, LowAt: a.lowAt, High: a.high, HighAt: a.highAt, Count: a.count}
	if lowerIsBetter {
		s.Best, s.BestAt, s.Worst, s.WorstAt = a.low, a.lowAt, a.high, a.highAt
	} else {
		s.Best, s.BestAt, s.Worst, s.WorstAt = a.high, a.highAt, a.low, a.lowAt
	}
	return s
}

type dayAccumulator struct{ download, upload, ping, jitter statAccumulator }

// StatsHandler — GET /api/stats?days=7&profile_id=0
func (s *AppState) StatsHandler(w http.ResponseWriter, r *http.Request) {
	days := parseQueryInt(r, "days", 7)
	if days != 3 && days != 5 && days != 7 && days != 14 && days != 30 {
		days = 7
	}
	profileID := int64(parseQueryInt(r, "profile_id", 0))

	now := time.Now()
	startLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	cutoff := startLocal.UTC()

	results, err := storage.GetResults(s.DB, profileID, 1000, cutoff, time.Time{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var dl, ul, ping, jitter statAccumulator
	byDay := map[string]*dayAccumulator{}
	included := 0
	for _, x := range results {
		if x.Status != "success" || x.MeasuredAt.Before(cutoff) {
			continue
		}
		included++
		dl.add(x.DownloadMbps, x.MeasuredAt)
		ul.add(x.UploadMbps, x.MeasuredAt)
		ping.add(x.PingMs, x.MeasuredAt)
		jitter.add(x.JitterMs, x.MeasuredAt)
		key := x.MeasuredAt.Local().Format("2006-01-02")
		d := byDay[key]
		if d == nil {
			d = &dayAccumulator{}
			byDay[key] = d
		}
		d.download.add(x.DownloadMbps, x.MeasuredAt)
		d.upload.add(x.UploadMbps, x.MeasuredAt)
		d.ping.add(x.PingMs, x.MeasuredAt)
		d.jitter.add(x.JitterMs, x.MeasuredAt)
	}

	// Immer die vollständige Kalenderachse liefern: ältester Tag links, heute rechts.
	// Tage ohne Messung bleiben mit Nullwerten bestehen und werden im Chart als Lücke gezeigt.
	keys := make([]string, 0, days)
	for i := 0; i < days; i++ {
		keys = append(keys, startLocal.AddDate(0, 0, i).Format("2006-01-02"))
	}
	daily := make([]dailyStats, 0, len(keys))
	for _, k := range keys {
		d := byDay[k]
		if d == nil {
			daily = append(daily, dailyStats{Date: k})
			continue
		}
		daily = append(daily, dailyStats{Date: k,
			Download: d.download.summary(false).Average, DownloadLow: d.download.low, DownloadHigh: d.download.high,
			Upload: d.upload.summary(false).Average, UploadLow: d.upload.low, UploadHigh: d.upload.high,
			Ping: d.ping.summary(true).Average, PingLow: d.ping.low, PingHigh: d.ping.high,
			Jitter: d.jitter.summary(true).Average, JitterLow: d.jitter.low, JitterHigh: d.jitter.high})
	}
	writeJSON(w, map[string]any{
		"days": days, "from": cutoff, "to": time.Now(), "result_count": included, "daily": daily,
		"metrics": map[string]metricSummary{
			"download": dl.summary(false), "upload": ul.summary(false),
			"ping": ping.summary(true), "jitter": jitter.summary(true),
		},
	})
}
