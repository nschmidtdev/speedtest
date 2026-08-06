package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"speedtest/internal/engine"
)

// CreateProfile legt ein neues Profil an.
func CreateProfile(db *sql.DB, p engine.Profile) (int64, error) {
	metricsJSON, err := metricsToJSON(p.Metrics)
	if err != nil {
		return 0, fmt.Errorf("invalid metrics: %w", err)
	}
	serverIDsJSON, err := json.Marshal(p.ServerIDs)
	if err != nil {
		serverIDsJSON = []byte("[]")
	}
	mode := p.ServerMode
	if mode == "" {
		mode = "auto"
	}

	res, err := db.Exec(
		`INSERT INTO profiles (name, description, metrics, cron_expr, enabled, server_id, server_mode, server_ids)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.Description, metricsJSON, p.CronExpr, p.Enabled, p.ServerID, mode, string(serverIDsJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create profile: %w", err)
	}

	return res.LastInsertId()
}

// GetProfile liefert ein einzelnes Profil anhand der ID.
func GetProfile(db *sql.DB, id int64) (*engine.Profile, error) {
	row := db.QueryRow(`SELECT id, name, description, metrics, cron_expr, enabled, server_id, server_mode, server_ids FROM profiles WHERE id = ?`, id)
	return scanProfile(row)
}

// ListProfiles liefert alle Profile.
func ListProfiles(db *sql.DB) ([]engine.Profile, error) {
	rows, err := db.Query(`SELECT id, name, description, metrics, cron_expr, enabled, server_id, server_mode, server_ids FROM profiles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list profiles: %w", err)
	}
	defer rows.Close()

	var profiles []engine.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, nil
}

// UpdateProfile aktualisiert ein bestehendes Profil.
func UpdateProfile(db *sql.DB, p engine.Profile) error {
	metricsJSON, err := metricsToJSON(p.Metrics)
	if err != nil {
		return fmt.Errorf("invalid metrics: %w", err)
	}
	serverIDsJSON, err := json.Marshal(p.ServerIDs)
	if err != nil {
		serverIDsJSON = []byte("[]")
	}
	mode := p.ServerMode
	if mode == "" {
		mode = "auto"
	}

	_, err = db.Exec(
		`UPDATE profiles SET name=?, description=?, metrics=?, cron_expr=?, enabled=?, server_id=?, server_mode=?, server_ids=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Name, p.Description, metricsJSON, p.CronExpr, p.Enabled, p.ServerID, mode, string(serverIDsJSON), p.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	return nil
}

// DeleteProfile löscht ein Profil und alle verknüpften Daten.
// Results und Tariffs werden zuerst gelöscht, damit der FK-Constraint
// (foreign_keys=ON) nicht zuschlägt.
func DeleteProfile(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Results zuerst (sie referenzieren tariffs über tariff_id)
	if _, err := tx.Exec(`DELETE FROM results WHERE profile_id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete results: %w", err)
	}
	// Tariffs
	if _, err := tx.Exec(`DELETE FROM tariffs WHERE profile_id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete tariffs: %w", err)
	}
	// Profile
	if _, err := tx.Exec(`DELETE FROM profiles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}
	return tx.Commit()
}

// SetProfileEnabled aktiviert/deaktiviert ein Profil.
func SetProfileEnabled(db *sql.DB, id int64, enabled bool) error {
	_, err := db.Exec(`UPDATE profiles SET enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, enabled, id)
	if err != nil {
		return fmt.Errorf("failed to toggle profile: %w", err)
	}
	return nil
}

// === Helpers ===

// scanner interface — works for both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanProfile(s scanner) (*engine.Profile, error) {
	var p engine.Profile
	var metricsJSON string
	var cronExpr sql.NullString
	var desc sql.NullString
	var serverMode sql.NullString
	var serverIDsJSON sql.NullString

	err := s.Scan(&p.ID, &p.Name, &desc, &metricsJSON, &cronExpr, &p.Enabled, &p.ServerID, &serverMode, &serverIDsJSON)
	if err != nil {
		return nil, err
	}

	p.Description = desc.String
	p.CronExpr = cronExpr.String
	p.ServerMode = serverMode.String
	if p.ServerMode == "" {
		p.ServerMode = "auto"
	}
	if serverIDsJSON.Valid && serverIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(serverIDsJSON.String), &p.ServerIDs)
	}
	if p.ServerIDs == nil {
		p.ServerIDs = []int{}
	}
	p.Metrics, err = jsonToMetrics(metricsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics JSON: %w", err)
	}

	return &p, nil
}
