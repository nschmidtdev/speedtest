package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS profiles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    metrics     TEXT NOT NULL DEFAULT '["download","upload","ping","jitter"]',
    cron_expr   TEXT,
    enabled     BOOLEAN NOT NULL DEFAULT 1,
    server_id   INTEGER DEFAULT 0,
    server_mode TEXT NOT NULL DEFAULT 'auto',
    server_ids  TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS results (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id      INTEGER REFERENCES profiles(id),
    tariff_id       INTEGER REFERENCES tariffs(id),
    tariff_down_percent        REAL,
    tariff_down_deviation_mbps REAL,
    tariff_down_status         TEXT,
    tariff_up_percent          REAL,
    tariff_up_deviation_mbps   REAL,
    tariff_up_status           TEXT,
    measured_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    download_mbps   REAL,
    upload_mbps     REAL,
    ping_ms         REAL,
    jitter_ms       REAL,
    bufferbloat_idle_ms   REAL,
    bufferbloat_loaded_ms REAL,
    bufferbloat_score     TEXT,
    packet_loss_pct       REAL,
    traceroute            TEXT,
    server_name     TEXT,
    server_url      TEXT,
    duration_ms     INTEGER,
    status          TEXT NOT NULL DEFAULT 'success',
    error_message   TEXT,
    failed_metrics  TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS tariffs (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id            INTEGER NOT NULL REFERENCES profiles(id),
    provider              TEXT NOT NULL,
    name                  TEXT NOT NULL,
    access_technology     TEXT,
    advertised_down_mbps  REAL NOT NULL,
    advertised_up_mbps    REAL NOT NULL,
    normal_down_mbps      REAL,
    normal_up_mbps        REAL,
    minimum_down_mbps     REAL,
    minimum_up_mbps       REAL,
    valid_from            DATETIME NOT NULL,
    valid_to              DATETIME,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tariffs_profile_period ON tariffs(profile_id, valid_from DESC);
CREATE INDEX IF NOT EXISTS idx_results_profile_time ON results(profile_id, measured_at DESC);
CREATE INDEX IF NOT EXISTS idx_results_time ON results(measured_at DESC);

CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL DEFAULT '',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Open öffnet die SQLite-Datenbank und führt Migrationen aus.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite concurrency: single writer, multiple readers via WAL
	db.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Warning: could not enable WAL mode: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		log.Printf("Warning: could not enable foreign keys: %v", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// schema_version tracken, damit teure Migrationen nur einmal laufen
	if err := migrateSchemaVersions(db); err != nil {
		return nil, fmt.Errorf("schema version migration failed: %w", err)
	}

	// Migration: add server_mode and server_ids columns if they don't exist
	// (ALTER TABLE ADD COLUMN errors if column exists, so we ignore the error)
	for _, col := range []struct{ name, def string }{
		{"server_mode", "TEXT NOT NULL DEFAULT 'auto'"},
		{"server_ids", "TEXT NOT NULL DEFAULT '[]'"},
	} {
		_, _ = db.Exec(fmt.Sprintf("ALTER TABLE profiles ADD COLUMN %s %s", col.name, col.def))
	}
	if _, err := db.Exec("ALTER TABLE results ADD COLUMN tariff_id INTEGER REFERENCES tariffs(id)"); err != nil {
		// Column likely already exists — ignore
	}
	for _, col := range []struct{ name, def string }{
		{"tariff_down_percent", "REAL"},
		{"tariff_down_deviation_mbps", "REAL"},
		{"tariff_down_status", "TEXT"},
		{"tariff_up_percent", "REAL"},
		{"tariff_up_deviation_mbps", "REAL"},
		{"tariff_up_status", "TEXT"},
		{"failed_metrics", "TEXT NOT NULL DEFAULT '[]'"},
	} {
		_, _ = db.Exec(fmt.Sprintf("ALTER TABLE results ADD COLUMN %s %s", col.name, col.def))
	}

	// Retention: clean up old results if SPEEDTEST_RETENTION_DAYS is set
	if retentionDays := getRetentionDays(); retentionDays > 0 {
		if deleted, err := DeleteOldResults(db, retentionDays); err != nil {
			log.Printf("Warning: retention cleanup failed: %v", err)
		} else if deleted > 0 {
			log.Printf("Retention: deleted %d results older than %d days", deleted, retentionDays)
		}
	}

	return db, nil
}

func getRetentionDays() int {
	v := os.Getenv("SPEEDTEST_RETENTION_DAYS")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// currentSchemaVersion wird inkrementiert, wenn eine neue versionierte Migration dazukommt.
const currentSchemaVersion = 2

// migrateSchemaVersions führt versionierte Migrationen aus, die nur einmal
// (beim Sprung von version N auf N+1) laufen sollen. Das verhindert, dass
// teure Backfills bei jedem Start wiederholt werden.
func migrateSchemaVersions(db *sql.DB) error {
	// Tabelle anlegen falls nicht vorhanden (IF NOT EXISTS im schema-Block nicht garantiert
	// wenn schema_version dort noch nicht steht)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var current int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for v := current + 1; v <= currentSchemaVersion; v++ {
		switch v {
		case 1:
			// v1: Backfill tariff comparison snapshots (war vorher bei jedem Start)
			if err := BackfillTariffComparisons(db); err != nil {
				return fmt.Errorf("migration v1 (backfill tariff comparisons): %w", err)
			}
		case 2:
			// v2: failed_metrics column wurde bereits per ALTER TABLE hinzugefügt.
			// Keine zusätzliche Aktion nötig — Marker-Version für zukünftige Erweiterungen.
		}
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, v); err != nil {
			return fmt.Errorf("record schema version %d: %w", v, err)
		}
		log.Printf("Schema migration v%d applied", v)
	}

	return nil
}

// SeedDefaults legt die Standard-Profile an, falls noch keine existieren.
func SeedDefaults(db *sql.DB) error {
	defaults := []struct {
		name, desc, metrics, cron string
		enabled                   bool
	}{
		{"Internet-Full", "Volle Internet-Messung", `["download","upload","ping","jitter","bufferbloat"]`, "0 0 */1 * * *", true},
		{"Internet-Quick", "Häufige Ausfall-Überwachung", `["ping","jitter"]`, "0 */5 * * * *", true},
		{"Traceroute", "Routing-Veränderungen tracken", `["traceroute"]`, "0 0 */6 * * *", false},
	}

	for _, d := range defaults {
		// INSERT OR IGNORE — falls der Name schon existiert, überspringen
		_, err := db.Exec(
			`INSERT OR IGNORE INTO profiles (name, description, metrics, cron_expr, enabled) VALUES (?, ?, ?, ?, ?)`,
			d.name, d.desc, d.metrics, d.cron, d.enabled,
		)
		if err != nil {
			return fmt.Errorf("failed to seed profile %s: %w", d.name, err)
		}
	}

	return nil
}

// metricsToJSON konvertiert ein []string zu JSON (für DB-Speicherung).
func metricsToJSON(metrics []string) (string, error) {
	b, err := json.Marshal(metrics)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// jsonToMetrics parst ein JSON-Array zu []string.
func jsonToMetrics(s string) ([]string, error) {
	if s == "" {
		return []string{}, nil
	}
	var metrics []string
	if err := json.Unmarshal([]byte(s), &metrics); err != nil {
		return []string{}, err
	}
	return metrics, nil
}
