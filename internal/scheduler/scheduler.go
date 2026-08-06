package scheduler

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"speedtest/internal/engine"
	"speedtest/internal/storage"
)

var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// ValidateExpr prüft einen sechsstelligen Cron-Ausdruck (Sek Min Std Tag Mon Wochentag).
func ValidateExpr(expr string) error {
	if expr == "" {
		return nil
	}
	if _, err := cronParser.Parse(expr); err != nil {
		return fmt.Errorf("ungültiger Cron-Ausdruck: %w", err)
	}
	return nil
}

// TestRunner ist das Interface, das der Scheduler aufruft, um einen Test auszuführen.
// Es wird von AppState implementiert (shared logic).
type TestRunner interface {
	RunScheduledTest(profileID int64)
}

// JobStatus repräsentiert den Zustand eines Scheduler-Jobs fürs Frontend.
type JobStatus struct {
	ProfileID   int64      `json:"profile_id"`
	ProfileName string     `json:"profile_name"`
	CronExpr    string     `json:"cron_expr"`
	Enabled     bool       `json:"enabled"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	LastRun     *time.Time `json:"last_run,omitempty"`
}

// Scheduler verwaltet cron-gesteuerte Speedtests.
type Scheduler struct {
	cron   *cron.Cron
	db     *sql.DB
	runner TestRunner

	mu       sync.Mutex
	entryMap map[int64]cron.EntryID // profileID → cron entry
	lastRun  map[int64]time.Time    // profileID → last execution time
}

// New erstellt einen neuen Scheduler.
func New(db *sql.DB, runner TestRunner) *Scheduler {
	return &Scheduler{
		cron: cron.New(
			cron.WithLogger(cron.PrintfLogger(log.New(cronLogWriter{}, "scheduler: ", log.LstdFlags))),
			cron.WithSeconds(), // 6-field cron: sec min hour dom mon dow
		),
		db:       db,
		runner:   runner,
		entryMap: make(map[int64]cron.EntryID),
		lastRun:  make(map[int64]time.Time),
	}
}

// Start lädt alle aktivierten Profile aus der DB und startet den Cron-Scheduler.
func (s *Scheduler) Start() error {
	profiles, err := storage.ListProfiles(s.db)
	if err != nil {
		return err
	}

	for _, p := range profiles {
		if p.Enabled && p.CronExpr != "" {
			s.addJob(p)
		}
	}

	s.cron.Start()
	log.Printf("Scheduler started with %d active jobs", len(s.entryMap))
	return nil
}

// Stop hält den Scheduler an.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// SyncProfile aktualisiert einen einzelnen Profil-Job im Scheduler.
// Wird bei Create/Update/Toggle/Delete eines Profils aufgerufen.
func (s *Scheduler) SyncProfile(p engine.Profile) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing job if present
	if entryID, ok := s.entryMap[p.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entryMap, p.ID)
	}

	// Add new job if enabled and has cron expr
	if p.Enabled && p.CronExpr != "" {
		s.addJob(p)
	}
}

// RemoveProfile entfernt einen Profil-Job aus dem Scheduler.
func (s *Scheduler) RemoveProfile(profileID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entryMap[profileID]; ok {
		s.cron.Remove(entryID)
		delete(s.entryMap, profileID)
	}
}

// Status liefert die aktuellen Job-States für alle Profile.
func (s *Scheduler) Status() []JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := storage.ListProfiles(s.db)
	if err != nil {
		log.Printf("scheduler status: failed to list profiles: %v", err)
		return []JobStatus{}
	}

	result := make([]JobStatus, 0, len(profiles))
	for _, p := range profiles {
		js := JobStatus{
			ProfileID:   p.ID,
			ProfileName: p.Name,
			CronExpr:    p.CronExpr,
			Enabled:     p.Enabled,
		}

		if entryID, ok := s.entryMap[p.ID]; ok {
			entry := s.cron.Entry(entryID)
			if !entry.Next.IsZero() {
				n := entry.Next
				js.NextRun = &n
			}
		}

		if lr, ok := s.lastRun[p.ID]; ok {
			lrCopy := lr
			js.LastRun = &lrCopy
		}

		result = append(result, js)
	}

	return result
}

// addJob registriert einen Cron-Job für ein Profil. Caller muss s.mu halten.
func (s *Scheduler) addJob(p engine.Profile) {
	profileID := p.ID
	metrics := p.Metrics

	entryID, err := s.cron.AddFunc(p.CronExpr, func() {
		log.Printf("Scheduler: running profile %d (%s)", profileID, p.Name)
		s.mu.Lock()
		s.lastRun[profileID] = time.Now()
		s.mu.Unlock()

		// Defer the actual test to the runner (AppState)
		s.runner.RunScheduledTest(profileID)

		_ = metrics // metrics are read by the runner from DB
	})
	if err != nil {
		log.Printf("Scheduler: failed to add job for profile %d (%s): %v", p.ID, p.CronExpr, err)
		return
	}

	s.entryMap[p.ID] = entryID
	log.Printf("Scheduler: registered profile %d (%s) with cron '%s', next run: %s",
		p.ID, p.Name, p.CronExpr, s.cron.Entry(entryID).Next.Format("15:04:05"))
}

// cronLogWriter unterdrückt die Standard-cron-Logs (leerer Writer).
type cronLogWriter struct{}

func (cronLogWriter) Write(p []byte) (int, error) { return len(p), nil }
