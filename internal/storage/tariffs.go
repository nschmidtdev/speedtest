package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"speedtest/internal/tariff"
)

func CreateTariffVersion(db *sql.DB, plan tariff.Tariff) (int64, error) {
	if err := tariff.Validate(plan); err != nil {
		return 0, err
	}
	if plan.ValidFrom.IsZero() {
		plan.ValidFrom = time.Now().UTC()
	} else {
		plan.ValidFrom = plan.ValidFrom.UTC()
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var latest string
	err = tx.QueryRow(`SELECT valid_from FROM tariffs WHERE profile_id = ? ORDER BY valid_from DESC LIMIT 1`, plan.ProfileID).Scan(&latest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to inspect tariff history: %w", err)
	}
	if err == nil {
		latestTime, parseErr := parseTariffTime(latest)
		if parseErr != nil {
			return 0, parseErr
		}
		if !plan.ValidFrom.After(latestTime) {
			return 0, errors.New("valid_from must be after the current tariff version")
		}
	}

	validFrom := plan.ValidFrom.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE tariffs SET valid_to = ? WHERE profile_id = ? AND valid_to IS NULL`, validFrom, plan.ProfileID); err != nil {
		return 0, fmt.Errorf("failed to close current tariff: %w", err)
	}
	res, err := tx.Exec(`INSERT INTO tariffs
		(profile_id, provider, name, access_technology, advertised_down_mbps, advertised_up_mbps,
		 normal_down_mbps, normal_up_mbps, minimum_down_mbps, minimum_up_mbps, valid_from)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ProfileID, plan.Provider, plan.Name, plan.AccessTechnology,
		plan.AdvertisedDownMbps, plan.AdvertisedUpMbps,
		nullFloat(plan.NormalDownMbps), nullFloat(plan.NormalUpMbps),
		nullFloat(plan.MinimumDownMbps), nullFloat(plan.MinimumUpMbps), validFrom)
	if err != nil {
		return 0, fmt.Errorf("failed to create tariff: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func ListActiveTariffs(db *sql.DB) ([]tariff.Tariff, error) {
	rows, err := db.Query(`SELECT t.id, t.profile_id, p.name, t.provider, t.name, t.access_technology,
		t.advertised_down_mbps, t.advertised_up_mbps, t.normal_down_mbps, t.normal_up_mbps,
		t.minimum_down_mbps, t.minimum_up_mbps, t.valid_from, t.valid_to, t.created_at
		FROM tariffs t JOIN profiles p ON p.id = t.profile_id
		WHERE t.valid_to IS NULL ORDER BY p.name, t.valid_from DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tariffs []tariff.Tariff
	for rows.Next() {
		plan, err := scanTariff(rows)
		if err != nil {
			return nil, err
		}
		tariffs = append(tariffs, *plan)
	}
	return tariffs, rows.Err()
}

func GetTariffByID(db *sql.DB, id int64) (*tariff.Tariff, error) {
	row := db.QueryRow(`SELECT t.id, t.profile_id, p.name, t.provider, t.name, t.access_technology,
		t.advertised_down_mbps, t.advertised_up_mbps, t.normal_down_mbps, t.normal_up_mbps,
		t.minimum_down_mbps, t.minimum_up_mbps, t.valid_from, t.valid_to, t.created_at
		FROM tariffs t JOIN profiles p ON p.id = t.profile_id WHERE t.id = ?`, id)
	return scanTariff(row)
}

func GetActiveTariff(db *sql.DB, profileID int64, at time.Time) (*tariff.Tariff, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	row := db.QueryRow(`SELECT t.id, t.profile_id, p.name, t.provider, t.name, t.access_technology,
		t.advertised_down_mbps, t.advertised_up_mbps, t.normal_down_mbps, t.normal_up_mbps,
		t.minimum_down_mbps, t.minimum_up_mbps, t.valid_from, t.valid_to, t.created_at
		FROM tariffs t JOIN profiles p ON p.id = t.profile_id
		WHERE t.profile_id = ? AND t.valid_from <= ? AND (t.valid_to IS NULL OR t.valid_to > ?)
		ORDER BY t.valid_from DESC LIMIT 1`, profileID, stamp, stamp)
	return scanTariff(row)
}

func scanTariff(s scanner) (*tariff.Tariff, error) {
	var plan tariff.Tariff
	var technology sql.NullString
	var normalDown, normalUp, minimumDown, minimumUp sql.NullFloat64
	var validFrom, createdAt string
	var validTo sql.NullString
	if err := s.Scan(&plan.ID, &plan.ProfileID, &plan.ProfileName, &plan.Provider, &plan.Name, &technology,
		&plan.AdvertisedDownMbps, &plan.AdvertisedUpMbps, &normalDown, &normalUp,
		&minimumDown, &minimumUp, &validFrom, &validTo, &createdAt); err != nil {
		return nil, err
	}
	plan.AccessTechnology = technology.String
	plan.NormalDownMbps = normalDown.Float64
	plan.NormalUpMbps = normalUp.Float64
	plan.MinimumDownMbps = minimumDown.Float64
	plan.MinimumUpMbps = minimumUp.Float64
	var err error
	plan.ValidFrom, err = parseTariffTime(validFrom)
	if err != nil {
		return nil, err
	}
	plan.CreatedAt, _ = parseTariffTime(createdAt)
	if validTo.Valid {
		parsed, err := parseTariffTime(validTo.String)
		if err != nil {
			return nil, err
		}
		plan.ValidTo = &parsed
	}
	return &plan, nil
}

func parseTariffTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid tariff timestamp %q", value)
}

func nullFloat(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}
