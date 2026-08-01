package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"speedtest/internal/engine"
	"speedtest/internal/scheduler"
	"speedtest/internal/storage"
)

// AppState hält alle geteilten Abhängigkeiten.
type AppState struct {
	DB     *sql.DB
	Engine *engine.SpeedtestEngine

	// WebSocket Broadcaster
	Broadcaster *Broadcaster

	// Scheduler
	Scheduler *scheduler.Scheduler

	// Test-Lock: nur ein Test gleichzeitig
	testMu      sync.Mutex
	testRunning bool
}

// NewAppState erzeugt den geteilten State.
func NewAppState(db *sql.DB, eng *engine.SpeedtestEngine) *AppState {
	return &AppState{
		DB:          db,
		Engine:      eng,
		Broadcaster: NewBroadcaster(),
	}
}

// RunScheduledTest implementiert scheduler.TestRunner.
// Wird vom Cron-Scheduler aufgerufen, wenn ein Profil fällig ist.
func (s *AppState) RunScheduledTest(profileID int64) {
	// Don't conflict with a manual test
	if !s.testMu.TryLock() {
		log.Printf("Scheduler: skipping profile %d — test already running", profileID)
		return
	}
	defer s.testMu.Unlock()
	s.testRunning = true
	defer func() { s.testRunning = false }()

	p, err := storage.GetProfile(s.DB, profileID)
	if err != nil {
		log.Printf("Scheduler: profile %d not found: %v", profileID, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := s.Engine.RunTest(ctx, engine.RunOptions{
		Metrics:    p.Metrics,
		ServerID:   p.ServerID,
		ServerMode: p.ServerMode,
		ServerIDs:  p.ServerIDs,
	}, func(ev engine.ProgressEvent) {
		ev.Profile = p.Name
		s.Broadcaster.Broadcast(ev)
	})

	if err != nil {
		log.Printf("Scheduler: test failed for profile %d: %v", profileID, err)
		return
	}

	result.ProfileID = profileID
	if _, err := storage.InsertResult(s.DB, *result); err != nil {
		log.Printf("Scheduler: failed to save result: %v", err)
	}
}

// === Helpers ===

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("Failed to write JSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseQueryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// === Handlers ===

// HealthHandler — GET /api/health
func (s *AppState) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok", "service": "speedtest"})
}

// ListProfilesHandler — GET /api/profiles
func (s *AppState) ListProfilesHandler(w http.ResponseWriter, r *http.Request) {
	profiles, err := storage.ListProfiles(s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if profiles == nil {
		profiles = []engine.Profile{}
	}
	writeJSON(w, profiles)
}

// CreateProfileHandler — POST /api/profiles
func (s *AppState) CreateProfileHandler(w http.ResponseWriter, r *http.Request) {
	var p engine.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := scheduler.ValidateExpr(p.CronExpr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := storage.CreateProfile(s.DB, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.ID = id
	// Sync scheduler if enabled
	if s.Scheduler != nil && p.Enabled && p.CronExpr != "" {
		s.Scheduler.SyncProfile(p)
	}
	writeJSON(w, p)
}

// ProfileDetailHandler — PUT/DELETE /api/profiles/{id}
func (s *AppState) ProfileDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /api/profiles/{id}
	idStr := r.URL.Path[len("/api/profiles/"):]
	// Handle sub-paths like /api/profiles/{id}/enable
	if idx := indexByte(idStr, '/'); idx >= 0 {
		idStr = idStr[:idx]
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := storage.GetProfile(s.DB, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeJSON(w, p)

	case http.MethodPut:
		var p engine.Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		p.ID = id
		if err := scheduler.ValidateExpr(p.CronExpr); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := storage.UpdateProfile(s.DB, p); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Sync scheduler
		if s.Scheduler != nil {
			s.Scheduler.SyncProfile(p)
		}
		writeJSON(w, p)

	case http.MethodDelete:
		if err := storage.DeleteProfile(s.DB, id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Remove from scheduler
		if s.Scheduler != nil {
			s.Scheduler.RemoveProfile(id)
		}
		writeJSON(w, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ProfileToggleHandler — POST /api/profiles/{id}/enable|disable
func (s *AppState) ProfileToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := r.URL.Path
	var idStr string
	var enable bool

	if endsWith(path, "/enable") {
		idStr = path[len("/api/profiles/") : len(path)-len("/enable")]
		enable = true
	} else if endsWith(path, "/disable") {
		idStr = path[len("/api/profiles/") : len(path)-len("/disable")]
		enable = false
	} else {
		writeError(w, http.StatusBadRequest, "invalid toggle path")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	if err := storage.SetProfileEnabled(s.DB, id, enable); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync scheduler
	if s.Scheduler != nil {
		p, err := storage.GetProfile(s.DB, id)
		if err == nil {
			s.Scheduler.SyncProfile(*p)
		}
	}

	writeJSON(w, map[string]any{"id": id, "enabled": enable})
}

// RunTestHandler — POST /api/test/run
func (s *AppState) RunTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !s.testMu.TryLock() {
		writeError(w, http.StatusConflict, "a test is already running")
		return
	}
	defer s.testMu.Unlock()
	s.testRunning = true
	defer func() { s.testRunning = false }()

	var body struct {
		ProfileID int `json:"profile_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var metrics []string
	var serverID int
	var serverMode string
	var serverIDs []int

	if body.ProfileID > 0 {
		p, err := storage.GetProfile(s.DB, int64(body.ProfileID))
		if err != nil {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		metrics = p.Metrics
		serverID = p.ServerID
		serverMode = p.ServerMode
		serverIDs = p.ServerIDs
	}

	// Respond immediately — test runs async
	writeJSON(w, map[string]string{"status": "started"})

	// Run test in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		result, err := s.Engine.RunTest(ctx, engine.RunOptions{
			Metrics:    metrics,
			ServerID:   serverID,
			ServerMode: serverMode,
			ServerIDs:  serverIDs,
		}, func(ev engine.ProgressEvent) {
			s.Broadcaster.Broadcast(ev)
		})

		if err != nil {
			log.Printf("Test failed: %v", err)
			return
		}

		if body.ProfileID > 0 {
			result.ProfileID = int64(body.ProfileID)
		}
		if _, err := storage.InsertResult(s.DB, *result); err != nil {
			log.Printf("Failed to save result: %v", err)
		}
	}()
}

// TestStatusHandler — GET /api/test/status
func (s *AppState) TestStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"running": s.testRunning,
	})
}

// ResultsHandler — GET /api/results
func (s *AppState) ResultsHandler(w http.ResponseWriter, r *http.Request) {
	profileID := int64(parseQueryInt(r, "profile_id", 0))
	limit := parseQueryInt(r, "limit", 100)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	results, err := storage.GetResults(s.DB, profileID, limit, time.Time{}, time.Time{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if results == nil {
		results = []engine.TestResult{}
	}
	writeJSON(w, results)
}

// ServersHandler — GET /api/servers
func (s *AppState) ServersHandler(w http.ResponseWriter, r *http.Request) {
	limit := parseQueryInt(r, "limit", 20)
	servers, err := s.Engine.FindServers(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, servers)
}

// SchedulerStatusHandler — GET /api/scheduler/status
func (s *AppState) SchedulerStatusHandler(w http.ResponseWriter, r *http.Request) {
	if s.Scheduler == nil {
		writeJSON(w, []any{})
		return
	}
	status := s.Scheduler.Status()
	if status == nil {
		status = []scheduler.JobStatus{}
	}
	writeJSON(w, status)
}

// === Mini string helpers (avoid pulling strings just for HasSuffix/IndexByte) ===

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
