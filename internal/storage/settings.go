package storage

import (
	"database/sql"
	"fmt"
)

// ComplaintAddress fasst die Absender- und Empfängerdaten für das Beschwerdeschreiben.
type ComplaintAddress struct {
	// Absender
	FullName string `json:"complaint_name"`
	Street   string `json:"complaint_street"`
	City     string `json:"complaint_city"`
	Phone    string `json:"complaint_phone"`
	Email    string `json:"complaint_email"`
	// Empfänger (Anbieter-Sitz)
	ProviderStreet string `json:"complaint_provider_street"`
	ProviderCity   string `json:"complaint_provider_city"`
}

// complaintKeys sind die erlaubten Setting-Keys (Whitelist).
var complaintKeys = []string{
	"complaint_name", "complaint_street", "complaint_city",
	"complaint_phone", "complaint_email",
	"complaint_provider_street", "complaint_provider_city",
}

// GetSetting liest einen einzelnen Wert.
func GetSetting(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// GetAllSettings liest alle bekannten Settings als Map.
func GetAllSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// SetSetting schreibt einen Wert (Upsert).
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to set setting %s: %w", key, err)
	}
	return nil
}

// SetSettings schreibt mehrere Werte atomar.
func SetSettings(db *sql.DB, m map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range m {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("failed to set %s: %w", k, err)
		}
	}
	return tx.Commit()
}

// GetComplaintAddress lädt die Adress-Fields aus den Settings.
func GetComplaintAddress(db *sql.DB) (ComplaintAddress, error) {
	m, err := GetAllSettings(db)
	if err != nil {
		return ComplaintAddress{}, err
	}
	return ComplaintAddress{
		FullName:       m["complaint_name"],
		Street:         m["complaint_street"],
		City:           m["complaint_city"],
		Phone:          m["complaint_phone"],
		Email:          m["complaint_email"],
		ProviderStreet: m["complaint_provider_street"],
		ProviderCity:   m["complaint_provider_city"],
	}, nil
}

// IsAllowedSettingKey prüft, ob ein Key in der Whitelist steht.
func IsAllowedSettingKey(key string) bool {
	for _, k := range complaintKeys {
		if k == key {
			return true
		}
	}
	return false
}
