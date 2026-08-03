package storage

import (
	"database/sql"
	"testing"
	"time"

	"speedtest/internal/engine"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// insertResult helper — fügt eine erfolgreiche Messung ein.
func insertResult(t *testing.T, db *sql.DB, profileID, tariffID int64, when time.Time, down, up float64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO results
		(profile_id, tariff_id, measured_at, download_mbps, upload_mbps, ping_ms, jitter_ms, status)
		VALUES (?, ?, ?, ?, ?, 10, 1, 'success')`,
		profileID, tariffID, when.Format("2006-01-02 15:04:05"), down, up)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

// workdaysBack geht N Werktage (Mo–Sa) vom Startdatum zurück.
func workdaysBack(from time.Time, n int) time.Time {
	d := from
	count := 0
	for count < n {
		d = d.AddDate(0, 0, -1)
		if d.Weekday() != time.Sunday {
			count++
		}
	}
	return d
}

func TestComputeStreak_AllBelow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Heute und die letzten 3 Tage, jeden Tag 2 Messungen mit 50 Mbit/s
	// bei advertised 100 / normal 90 → alle Below.
	now := time.Now()
	for i := 0; i < 4; i++ {
		d := now.AddDate(0, 0, -i)
		insertResult(t, db, 1, 0, d, 50, 20)
		insertResult(t, db, 1, 0, time.Date(d.Year(), d.Month(), d.Day(), 18, 0, 0, 0, d.Location()), 50, 20)
	}

	res := ComputeStreak(db, 1, 90, 36, 100, 40, 14)
	// Mindestens die letzten 3 Werktage sollten Below sein.
	if res.CurrentStreak < 2 {
		t.Errorf("CurrentStreak = %d, want >= 2 (4 Tage mit 50 Mbit/s je below)", res.CurrentStreak)
	}
	if !res.ReadyToComplain {
		t.Errorf("ReadyToComplain = false, want true")
	}
	if res.TotalBelowDays < 3 {
		t.Errorf("TotalBelowDays = %d, want >= 3", res.TotalBelowDays)
	}
}

func TestComputeStreak_NoData(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	res := ComputeStreak(db, 1, 90, 36, 100, 40, 14)
	if res.CurrentStreak != 0 {
		t.Errorf("CurrentStreak = %d, want 0", res.CurrentStreak)
	}
	if res.ReadyToComplain {
		t.Errorf("ReadyToComplain = true, want false bei leerer DB")
	}
}

func TestComputeStreak_StreakBreaksOnGoodDay(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	now := time.Now()
	// Heute (Werktag 0): Below
	insertResult(t, db, 1, 0, now, 50, 20)
	// Vorheriger Werktag (-1): OK → MUSS Strähne brechen
	prevWd := workdaysBack(now, 1)
	insertResult(t, db, 1, 0, prevWd, 95, 38) // über Normalwert
	// Werktag -2: Below (dürfte nicht mehr zählen da Strähne gebrochen)
	prevWd2 := workdaysBack(now, 2)
	insertResult(t, db, 1, 0, prevWd2, 50, 20)

	res := ComputeStreak(db, 1, 90, 36, 100, 40, 14)
	if res.CurrentStreak != 1 {
		t.Errorf("CurrentStreak = %d, want 1 (guter Werktag unterbricht)", res.CurrentStreak)
	}
}

func TestComputeStreak_NormalFallback(t *testing.T) {
	// Wenn normalDown=0, soll 90 % von advertised als Schwelle gelten.
	db := openTestDB(t)
	defer db.Close()

	now := time.Now()
	// Heute und vorheriger Werktag: 85 Mbps (unter 90 % von 100 = 90)
	insertResult(t, db, 1, 0, now, 85, 30)
	prevWd := workdaysBack(now, 1)
	insertResult(t, db, 1, 0, prevWd, 85, 30)

	res := ComputeStreak(db, 1, 0, 0, 100, 40, 14) // kein Normalwert → Fallback 90 %
	if res.CurrentStreak < 2 {
		t.Errorf("CurrentStreak = %d, want >= 2 mit 90 %% Fallback (85 < 90)", res.CurrentStreak)
	}
	if !res.ReadyToComplain {
		t.Errorf("ReadyToComplain = false, want true bei 2 Below-Werktagen")
	}
}

func TestComputeStreak_SkipsSunday(t *testing.T) {
	// Sonntag sollte nicht als Werktag zählen und die Strähne nicht brechen.
	db := openTestDB(t)
	defer db.Close()

	now := time.Now()
	// Finde den letzten Sonntag
	var lastSunday time.Time
	for i := 0; i <= 7; i++ {
		d := now.AddDate(0, 0, -i)
		if d.Weekday() == time.Sunday {
			lastSunday = d
			break
		}
	}
	if lastSunday.IsZero() {
		t.Skip("konnte letzten Sonntag nicht bestimmen")
	}

	// Sonntag: Below, zählt aber nicht als Werktag.
	insertResult(t, db, 1, 0, lastSunday, 50, 20)

	res := ComputeStreak(db, 1, 90, 36, 100, 40, 14)
	// Sonntag darf nie als Werktag markiert sein.
	for _, d := range res.Days {
		if d.Date.Weekday() == time.Sunday && d.Workday {
			t.Errorf("Sonntag %v ist als Werktag markiert", d.Date)
		}
	}
	// Sonntag-Below darf nicht zur Strähne zählen.
	if res.CurrentStreak != 0 {
		t.Errorf("CurrentStreak = %d, want 0 (nur Sonntag hatte Below-Daten)", res.CurrentStreak)
	}
}

// Stellt sicher, dass ComputeStreak auch ohne Tarif-Daten in der DB funktioniert.
func TestComputeStreak_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	res := ComputeStreak(db, 999, 90, 36, 100, 40, 7)
	_ = res // darf nicht panic-en
}

// engine.TestResult wird referenziert um sicherzustellen dass die Typen stimmen.
var _ = engine.TestResult{}
