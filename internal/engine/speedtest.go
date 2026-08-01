package engine

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// SpeedtestEngine wrappt die speedtest-go Bibliothek.
type SpeedtestEngine struct {
	client *speedtest.Speedtest
}

// NewSpeedtestEngine erzeugt eine neue Engine-Instanz.
func NewSpeedtestEngine() *SpeedtestEngine {
	client := speedtest.New(
		speedtest.WithUserConfig(&speedtest.UserConfig{
			UserAgent:      "SpeedtestDashboard/0.1",
			PingMode:       speedtest.HTTP,
			MaxConnections: 8,
		}),
	)
	return &SpeedtestEngine{client: client}
}

// ServerInfo ist eine gekürzte Repräsentation eines Ookla-Servers fürs Frontend.
type ServerInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Country  string  `json:"country"`
	Sponsor  string  `json:"sponsor"`
	Distance float64 `json:"distance"`
}

// FindServers ruft verfügbare Server ab.
func (e *SpeedtestEngine) FindServers(limit int) ([]ServerInfo, error) {
	servers, err := e.client.FetchServers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	available := servers.Available()
	if available == nil || len(*available) == 0 {
		available = &servers
	}

	if limit > 0 && len(*available) > limit {
		*available = (*available)[:limit]
	}

	result := make([]ServerInfo, 0, len(*available))
	for _, s := range *available {
		result = append(result, ServerInfo{
			ID:       s.ID,
			Name:     s.Name,
			Country:  s.Country,
			Sponsor:  s.Sponsor,
			Distance: s.Distance,
		})
	}
	return result, nil
}

// RunOptions steuert, was gemessen wird.
type RunOptions struct {
	Metrics    []string // download, upload, ping, jitter
	ServerID   int      // 0 = auto-select
	ServerMode string   // "auto", "random", "fixed"
	ServerIDs  []int    // selected server IDs for random/fixed
}

// ProgressCallback wird während des Tests aufgerufen (für WebSocket-Push).
type ProgressCallback func(event ProgressEvent)

// RunTest führt einen Speedtest aus und liefert das Ergebnis.
func (e *SpeedtestEngine) RunTest(ctx context.Context, opts RunOptions, cb ProgressCallback) (*TestResult, error) {
	start := time.Now()
	result := &TestResult{
		MeasuredAt: start,
		Status:     "success",
	}
	if cb != nil {
		cb(ProgressEvent{Type: "test_start", Phase: "server", ProgressPct: 2, Timestamp: start})
		cb(ProgressEvent{Type: "progress", Phase: "server", ProgressPct: 5, Timestamp: time.Now()})
	}

	// Metrics-Set
	want := make(map[string]bool)
	for _, m := range opts.Metrics {
		want[m] = true
	}
	if len(want) == 0 {
		want["download"] = true
		want["upload"] = true
		want["ping"] = true
		want["jitter"] = true
	}

	// Server finden — abhängig vom ServerMode
	var server *speedtest.Server
	var err error

	switch {
	case opts.ServerMode == "fixed" && len(opts.ServerIDs) > 0:
		// Fixed: immer den ersten ausgewählten Server nehmen
		server, err = e.client.FetchServerByID(fmt.Sprintf("%d", opts.ServerIDs[0]))
	case opts.ServerMode == "random" && len(opts.ServerIDs) > 0:
		// Random: zufälligen Server aus der ausgewählten Liste
		idx := rng.Intn(len(opts.ServerIDs))
		server, err = e.client.FetchServerByID(fmt.Sprintf("%d", opts.ServerIDs[idx]))
	case opts.ServerID > 0:
		// Legacy: einzelne server_id
		server, err = e.client.FetchServerByID(fmt.Sprintf("%d", opts.ServerID))
	default:
		// Auto: Ookla wählt den nächsten Server
		servers, ferr := e.client.FetchServers()
		if ferr != nil {
			err = ferr
		} else {
			targets, terr := servers.FindServer(nil)
			if terr != nil || len(targets) == 0 {
				err = fmt.Errorf("no server found: %w", terr)
			} else {
				server = targets[0]
			}
		}
	}
	if err != nil || server == nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("server selection failed: %v", err)
		if cb != nil {
			cb(ProgressEvent{Type: "test_error", Error: result.ErrorMessage})
		}
		return result, err
	}

	result.ServerName = server.Name
	result.ServerURL = server.URL

	// === Ping + Jitter ===
	if want["ping"] || want["jitter"] {
		if cb != nil {
			cb(ProgressEvent{Type: "progress", Phase: "ping", ProgressPct: 12, Timestamp: time.Now()})
		}
		err = server.PingTest(func(latency time.Duration) {
			ms := float64(latency.Microseconds()) / 1000.0
			if cb != nil {
				cb(ProgressEvent{Type: "ping_update", PingMs: ms})
			}
		})
		if err != nil {
			log.Printf("Ping test warning: %v", err)
		} else {
			result.PingMs = float64(server.Latency.Microseconds()) / 1000.0
			result.JitterMs = float64(server.Jitter.Microseconds()) / 1000.0
			if cb != nil {
				cb(ProgressEvent{Type: "ping_update", PingMs: result.PingMs, JitterMs: result.JitterMs, ProgressPct: 22})
			}
		}
	}

	// === Download ===
	if want["download"] {
		if cb != nil {
			cb(ProgressEvent{Type: "progress", Phase: "download", ProgressPct: 28, Timestamp: time.Now()})
		}

		e.client.SetCallbackDownload(func(rate speedtest.ByteRate) {
			mbps := rate.Mbps()
			if cb != nil {
				cb(ProgressEvent{
					Type:        "progress",
					Phase:       "download",
					CurrentMbps: mbps,
				})
			}
		})

		err = server.DownloadTestContext(ctx)
		e.client.SetCallbackDownload(func(speedtest.ByteRate) {})
		if err != nil {
			log.Printf("Download test warning: %v", err)
		} else {
			result.DownloadMbps = server.DLSpeed.Mbps()
		}
	}

	// === Upload ===
	if want["upload"] {
		if cb != nil {
			cb(ProgressEvent{Type: "progress", Phase: "upload", ProgressPct: 58, Timestamp: time.Now()})
		}

		e.client.SetCallbackUpload(func(rate speedtest.ByteRate) {
			mbps := rate.Mbps()
			if cb != nil {
				cb(ProgressEvent{
					Type:        "progress",
					Phase:       "upload",
					CurrentMbps: mbps,
				})
			}
		})

		err = server.UploadTestContext(ctx)
		e.client.SetCallbackUpload(func(speedtest.ByteRate) {})
		if err != nil {
			log.Printf("Upload test warning: %v", err)
		} else {
			result.UploadMbps = server.ULSpeed.Mbps()
		}
	}

	// === Bufferbloat ===
	if want["bufferbloat"] {
		idleMs, loadedMs, score, bbErr := e.RunBufferbloat(ctx, cb)
		if bbErr != nil {
			log.Printf("Bufferbloat measurement warning: %v", bbErr)
		} else {
			result.BufferbloatIdleMs = idleMs
			result.BufferbloatLoadedMs = loadedMs
			result.BufferbloatScore = score
		}
	}

	// === Traceroute ===
	if want["traceroute"] {
		hops, trErr := e.RunTraceroute(ctx, "1.1.1.1", 30, cb)
		if trErr != nil {
			log.Printf("Traceroute warning: %v", trErr)
		} else {
			result.Traceroute = hops
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()

	if cb != nil {
		cb(ProgressEvent{Type: "test_complete", Result: result, Timestamp: time.Now()})
	}

	return result, nil
}
