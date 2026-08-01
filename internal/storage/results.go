package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"speedtest/internal/engine"
	"speedtest/internal/tariff"
)

const resultSelectColumns = `id, profile_id, tariff_id,
	tariff_down_percent, tariff_down_deviation_mbps, tariff_down_status,
	tariff_up_percent, tariff_up_deviation_mbps, tariff_up_status,
	measured_at, download_mbps, upload_mbps, ping_ms, jitter_ms,
	bufferbloat_idle_ms, bufferbloat_loaded_ms, bufferbloat_score,
	packet_loss_pct, traceroute, server_name, server_url, duration_ms, status, error_message`

// InsertResult speichert ein Testergebnis inklusive Tarif-Abweichungssnapshot.
func InsertResult(db *sql.DB, r engine.TestResult) (int64, error) {
	var tracerouteJSON any
	if len(r.Traceroute) > 0 {
		b, err := json.Marshal(r.Traceroute)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal traceroute: %w", err)
		}
		tracerouteJSON = string(b)
	}
	if r.TariffID == 0 && r.ProfileID > 0 && !r.MeasuredAt.IsZero() {
		stamp := r.MeasuredAt.UTC().Format(time.RFC3339Nano)
		_ = db.QueryRow(`SELECT id FROM tariffs WHERE profile_id = ? AND valid_from <= ?
			AND (valid_to IS NULL OR valid_to > ?) ORDER BY valid_from DESC LIMIT 1`,
			r.ProfileID, stamp, stamp).Scan(&r.TariffID)
	}
	if r.TariffID > 0 {
		plan, err := GetTariffByID(db, r.TariffID)
		if err != nil {
			return 0, fmt.Errorf("failed to load tariff snapshot: %w", err)
		}
		applyComparisonSnapshot(&r, tariff.Compare(r, *plan))
	}

	res, err := db.Exec(
		`INSERT INTO results
		    (profile_id, tariff_id, tariff_down_percent, tariff_down_deviation_mbps, tariff_down_status,
		     tariff_up_percent, tariff_up_deviation_mbps, tariff_up_status,
		     measured_at, download_mbps, upload_mbps, ping_ms, jitter_ms,
		     bufferbloat_idle_ms, bufferbloat_loaded_ms, bufferbloat_score,
		     packet_loss_pct, traceroute, server_name, server_url, duration_ms, status, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nilIfZero(r.ProfileID), nilIfZero(r.TariffID),
		comparisonNumber(r.TariffDownStatus, r.TariffDownPercent),
		comparisonNumber(r.TariffDownStatus, r.TariffDownDeviationMbps), nilIfEmpty(r.TariffDownStatus),
		comparisonNumber(r.TariffUpStatus, r.TariffUpPercent),
		comparisonNumber(r.TariffUpStatus, r.TariffUpDeviationMbps), nilIfEmpty(r.TariffUpStatus),
		r.MeasuredAt.UTC().Format("2006-01-02 15:04:05"),
		nilIfZeroFloat(r.DownloadMbps), nilIfZeroFloat(r.UploadMbps),
		nilIfZeroFloat(r.PingMs), nilIfZeroFloat(r.JitterMs),
		nilIfZeroFloat(r.BufferbloatIdleMs), nilIfZeroFloat(r.BufferbloatLoadedMs), nilIfEmpty(r.BufferbloatScore),
		nilIfZeroFloat(r.PacketLossPct), tracerouteJSON, r.ServerName, r.ServerURL,
		r.DurationMs, r.Status, r.ErrorMessage,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert result: %w", err)
	}
	return res.LastInsertId()
}

// GetResults liefert Ergebnisse mit optionalem Filter.
func GetResults(db *sql.DB, profileID int64, limit int, from, to time.Time) ([]engine.TestResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `SELECT ` + resultSelectColumns + ` FROM results WHERE 1=1`
	args := []any{}
	if profileID > 0 {
		query += ` AND profile_id = ?`
		args = append(args, profileID)
	}
	if !from.IsZero() {
		query += ` AND measured_at >= ?`
		args = append(args, from)
	}
	if !to.IsZero() {
		query += ` AND measured_at <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY measured_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query results: %w", err)
	}
	defer rows.Close()
	var results []engine.TestResult
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *r)
	}
	return results, rows.Err()
}

// GetResultByID liefert ein einzelnes Ergebnis einschließlich Tarif-Snapshot.
func GetResultByID(db *sql.DB, id int64) (*engine.TestResult, error) {
	return scanResult(db.QueryRow(`SELECT `+resultSelectColumns+` FROM results WHERE id = ?`, id))
}

// GetLatestResult liefert das neueste Ergebnis für ein Profil.
func GetLatestResult(db *sql.DB, profileID int64) (*engine.TestResult, error) {
	return scanResult(db.QueryRow(
		`SELECT `+resultSelectColumns+` FROM results WHERE profile_id = ? ORDER BY measured_at DESC LIMIT 1`,
		profileID,
	))
}

func scanResult(s scanner) (*engine.TestResult, error) {
	var r engine.TestResult
	var (
		profileID, tariffID, durationMs                         sql.NullInt64
		tariffDownPercent, tariffDownDeviation                  sql.NullFloat64
		tariffUpPercent, tariffUpDeviation                      sql.NullFloat64
		tariffDownStatus, tariffUpStatus                        sql.NullString
		downloadMbps, uploadMbps, pingMs, jitterMs              sql.NullFloat64
		bufferbloatIdleMs, bufferbloatLoadedMs, packetLossPct   sql.NullFloat64
		bufferbloatScore, tracerouteJSON, serverName, serverURL sql.NullString
		errorMessage, measuredAt                                sql.NullString
	)
	err := s.Scan(
		&r.ID, &profileID, &tariffID,
		&tariffDownPercent, &tariffDownDeviation, &tariffDownStatus,
		&tariffUpPercent, &tariffUpDeviation, &tariffUpStatus,
		&measuredAt, &downloadMbps, &uploadMbps, &pingMs, &jitterMs,
		&bufferbloatIdleMs, &bufferbloatLoadedMs, &bufferbloatScore,
		&packetLossPct, &tracerouteJSON, &serverName, &serverURL, &durationMs, &r.Status, &errorMessage,
	)
	if err != nil {
		return nil, err
	}
	r.ProfileID = profileID.Int64
	r.TariffID = tariffID.Int64
	r.TariffDownPercent = tariffDownPercent.Float64
	r.TariffDownDeviationMbps = tariffDownDeviation.Float64
	r.TariffDownStatus = tariffDownStatus.String
	r.TariffUpPercent = tariffUpPercent.Float64
	r.TariffUpDeviationMbps = tariffUpDeviation.Float64
	r.TariffUpStatus = tariffUpStatus.String
	r.DownloadMbps = downloadMbps.Float64
	r.UploadMbps = uploadMbps.Float64
	r.PingMs = pingMs.Float64
	r.JitterMs = jitterMs.Float64
	r.BufferbloatIdleMs = bufferbloatIdleMs.Float64
	r.BufferbloatLoadedMs = bufferbloatLoadedMs.Float64
	r.BufferbloatScore = bufferbloatScore.String
	r.PacketLossPct = packetLossPct.Float64
	r.ServerName = serverName.String
	r.ServerURL = serverURL.String
	r.DurationMs = durationMs.Int64
	r.ErrorMessage = errorMessage.String
	if tracerouteJSON.Valid && tracerouteJSON.String != "" {
		_ = json.Unmarshal([]byte(tracerouteJSON.String), &r.Traceroute)
	}
	if measuredAt.Valid {
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05", "2006-01-02T15:04:05",
			"2006-01-02 15:04:05.999999999-07:00", "2006-01-02T15:04:05.999999999Z07:00",
		} {
			if t, err := time.Parse(layout, measuredAt.String); err == nil {
				r.MeasuredAt = t
				break
			}
		}
	}
	return &r, nil
}

func nilIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nilIfZeroFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func comparisonNumber(status string, value float64) any {
	if status == "" || status == tariff.StatusInsufficientData {
		return nil
	}
	return value
}

func applyComparisonSnapshot(result *engine.TestResult, comparison tariff.Comparison) {
	result.TariffDownPercent = comparison.Download.Percent
	result.TariffDownDeviationMbps = comparison.Download.DeviationMbps
	result.TariffDownStatus = comparison.Download.Status
	result.TariffUpPercent = comparison.Upload.Percent
	result.TariffUpDeviationMbps = comparison.Upload.DeviationMbps
	result.TariffUpStatus = comparison.Upload.Status
}

// BackfillTariffComparisons persists derived values for older tariff-bound measurements.
func BackfillTariffComparisons(db *sql.DB) error {
	rows, err := db.Query(`SELECT r.id, r.download_mbps, r.upload_mbps,
		t.id, t.profile_id, t.provider, t.name, t.access_technology,
		t.advertised_down_mbps, t.advertised_up_mbps,
		t.normal_down_mbps, t.normal_up_mbps, t.minimum_down_mbps, t.minimum_up_mbps
		FROM results r JOIN tariffs t ON t.id = r.tariff_id
		WHERE r.tariff_down_status IS NULL OR r.tariff_up_status IS NULL`)
	if err != nil {
		return err
	}
	type candidate struct {
		result engine.TestResult
		plan   tariff.Tariff
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var download, upload, normalDown, normalUp, minimumDown, minimumUp sql.NullFloat64
		if err := rows.Scan(&c.result.ID, &download, &upload,
			&c.plan.ID, &c.plan.ProfileID, &c.plan.Provider, &c.plan.Name, &c.plan.AccessTechnology,
			&c.plan.AdvertisedDownMbps, &c.plan.AdvertisedUpMbps,
			&normalDown, &normalUp, &minimumDown, &minimumUp); err != nil {
			rows.Close()
			return err
		}
		c.result.DownloadMbps, c.result.UploadMbps = download.Float64, upload.Float64
		c.plan.NormalDownMbps, c.plan.NormalUpMbps = normalDown.Float64, normalUp.Float64
		c.plan.MinimumDownMbps, c.plan.MinimumUpMbps = minimumDown.Float64, minimumUp.Float64
		candidates = append(candidates, c)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, c := range candidates {
		comparison := tariff.Compare(c.result, c.plan)
		_, err := db.Exec(`UPDATE results SET
			tariff_down_percent = ?, tariff_down_deviation_mbps = ?, tariff_down_status = ?,
			tariff_up_percent = ?, tariff_up_deviation_mbps = ?, tariff_up_status = ? WHERE id = ?`,
			comparisonNumber(comparison.Download.Status, comparison.Download.Percent),
			comparisonNumber(comparison.Download.Status, comparison.Download.DeviationMbps), comparison.Download.Status,
			comparisonNumber(comparison.Upload.Status, comparison.Upload.Percent),
			comparisonNumber(comparison.Upload.Status, comparison.Upload.DeviationMbps), comparison.Upload.Status,
			c.result.ID)
		if err != nil {
			return err
		}
	}
	return nil
}
