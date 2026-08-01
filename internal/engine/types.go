package engine

import "time"

// TestResult repräsentiert ein einzelnes Messergebnis.
type TestResult struct {
	ID                      int64     `json:"id"`
	ProfileID               int64     `json:"profile_id"`
	TariffID                int64     `json:"tariff_id,omitempty"`
	TariffDownPercent       float64   `json:"tariff_down_percent,omitempty"`
	TariffDownDeviationMbps float64   `json:"tariff_down_deviation_mbps,omitempty"`
	TariffDownStatus        string    `json:"tariff_down_status,omitempty"`
	TariffUpPercent         float64   `json:"tariff_up_percent,omitempty"`
	TariffUpDeviationMbps   float64   `json:"tariff_up_deviation_mbps,omitempty"`
	TariffUpStatus          string    `json:"tariff_up_status,omitempty"`
	MeasuredAt              time.Time `json:"measured_at"`
	DownloadMbps            float64   `json:"download_mbps"`
	UploadMbps              float64   `json:"upload_mbps"`
	PingMs                  float64   `json:"ping_ms"`
	JitterMs                float64   `json:"jitter_ms"`
	BufferbloatIdleMs       float64   `json:"bufferbloat_idle_ms,omitempty"`
	BufferbloatLoadedMs     float64   `json:"bufferbloat_loaded_ms,omitempty"`
	BufferbloatScore        string    `json:"bufferbloat_score,omitempty"`
	PacketLossPct           float64   `json:"packet_loss_pct,omitempty"`
	Traceroute              []Hop     `json:"traceroute,omitempty"`
	ServerName              string    `json:"server_name"`
	ServerURL               string    `json:"server_url"`
	DurationMs              int64     `json:"duration_ms"`
	Status                  string    `json:"status"`
	ErrorMessage            string    `json:"error_message,omitempty"`
}

// Hop repräsentiert einen einzelnen Traceroute-Hop.
type Hop struct {
	TTL     int     `json:"ttl"`
	Address string  `json:"address"`
	Host    string  `json:"host,omitempty"`
	RttMs   float64 `json:"rtt_ms"`
}

// Profile definiert ein Test-Profil (welche Metriken, Cron, Server).
type Profile struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Metrics     []string `json:"metrics"`
	CronExpr    string   `json:"cron_expr"`
	Enabled     bool     `json:"enabled"`
	ServerID    int      `json:"server_id"`
	ServerMode  string   `json:"server_mode"` // "auto", "random", "fixed"
	ServerIDs   []int    `json:"server_ids"`  // selected server IDs for random/fixed
}

// ProgressEvent wird während eines laufenden Tests an Clients gepusht.
type ProgressEvent struct {
	Type        string      `json:"type"`
	Phase       string      `json:"phase,omitempty"`
	CurrentMbps float64     `json:"current_mbps,omitempty"`
	ProgressPct float64     `json:"progress_pct,omitempty"`
	PingMs      float64     `json:"ping_ms,omitempty"`
	JitterMs    float64     `json:"jitter_ms,omitempty"`
	Profile     string      `json:"profile,omitempty"`
	Timestamp   time.Time   `json:"timestamp,omitempty"`
	Result      *TestResult `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`
}
