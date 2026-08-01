package engine

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// RunBufferbloat misst die Latenz im Idle-Zustand und unter Last (simulierter Download).
// Gibt Idle-Ping, Loaded-Ping und einen Bufferbloat-Score zurück.
func (e *SpeedtestEngine) RunBufferbloat(ctx context.Context, cb ProgressCallback) (idleMs, loadedMs float64, score string, err error) {
	// === Phase 1: Idle Ping (10 Messungen) ===
	if cb != nil {
		cb(ProgressEvent{Type: "progress", Phase: "bufferbloat_idle", Timestamp: time.Now()})
	}

	idlePings, err := icmpPings(ctx, "1.1.1.1", 10, 100*time.Millisecond)
	if err != nil || len(idlePings) == 0 {
		// ICMP needs raw sockets (admin/root) — fallback to HTTP ping
		log.Printf("Bufferbloat: ICMP ping failed (%v), falling back to HTTP ping", err)
		idlePings, err = httpPings(ctx, "1.1.1.1", 10, 100*time.Millisecond)
		if err != nil || len(idlePings) == 0 {
			return 0, 0, "error", fmt.Errorf("bufferbloat idle measurement failed: %w", err)
		}
	}

	idleMs = median(idlePings)
	if cb != nil {
		cb(ProgressEvent{Type: "ping_update", PingMs: idleMs, Timestamp: time.Now()})
	}

	// === Phase 2: Loaded Ping (während Download läuft) ===
	if cb != nil {
		cb(ProgressEvent{Type: "progress", Phase: "bufferbloat_loaded", Timestamp: time.Now()})
	}

	// Start a download in background to saturate the link
	var wg sync.WaitGroup
	loadedPings := []float64{}
	var pingMu sync.Mutex

	ctxLoaded, cancelLoaded := context.WithTimeout(ctx, 15*time.Second)
	defer cancelLoaded()

	// Launch background download goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		// TCP connection flood: open multiple connections and download from the speedtest server
		servers, serr := e.client.FetchServers()
		if serr != nil || len(servers) == 0 {
			return
		}
		targets, terr := servers.FindServer(nil)
		if terr != nil || len(targets) == 0 {
			return
		}
		// We don't need the result, just the load
		_ = targets[0].DownloadTestContext(ctxLoaded)
	}()

	// While download runs, measure ping every 200ms
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctxLoaded.Done():
			goto done
		case <-ticker.C:
			p, perr := icmpPings(ctx, "1.1.1.1", 3, 50*time.Millisecond)
			if perr != nil || len(p) == 0 {
				p, perr = httpPings(ctx, "1.1.1.1", 3, 50*time.Millisecond)
				if perr != nil {
					continue
				}
			}
			m := median(p)
			pingMu.Lock()
			loadedPings = append(loadedPings, m)
			pingMu.Unlock()
			if cb != nil {
				cb(ProgressEvent{Type: "ping_update", PingMs: m, Timestamp: time.Now()})
			}
		}
	}

done:
	wg.Wait()

	if len(loadedPings) == 0 {
		return idleMs, 0, "error", fmt.Errorf("no loaded measurements collected")
	}

	loadedMs = median(loadedPings)

	// Score: latency increase (bloat) = loadedMs - idleMs
	bloat := loadedMs - idleMs
	switch {
	case bloat < 10:
		score = "A"
	case bloat < 20:
		score = "B"
	case bloat < 50:
		score = "C"
	case bloat < 100:
		score = "D"
	default:
		score = "F"
	}

	return idleMs, loadedMs, score, nil
}

// === ICMP Ping ===
func icmpPings(ctx context.Context, host string, count int, interval time.Duration) ([]float64, error) {
	// Note: ICMP requires raw sockets (root/Admin on most systems).
	// We try, and if it fails, the caller falls back to HTTP ping.
	pings := make([]float64, 0, count)
	conn, err := net.DialTimeout("ip4:icmp", host, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	icmpMsg := makeICMPEcho()

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return pings, ctx.Err()
		default:
		}

		conn.SetDeadline(time.Now().Add(2 * time.Second))
		start := time.Now()

		if _, err := conn.Write(icmpMsg); err != nil {
			continue
		}
		resp := make([]byte, 1500)
		n, err := conn.Read(resp)
		if err != nil || n < 28 {
			continue
		}
		elapsed := time.Since(start)
		pings = append(pings, float64(elapsed.Microseconds())/1000.0)

		if i < count-1 {
			time.Sleep(interval)
		}
	}

	if len(pings) == 0 {
		return nil, fmt.Errorf("no successful ICMP pings")
	}
	return pings, nil
}

func makeICMPEcho() []byte {
	// ICMP Echo Request: Type 8, Code 0
	msg := make([]byte, 16)
	msg[0] = 8 // Type: Echo Request
	msg[1] = 0 // Code
	msg[2] = 0 // Checksum (filled below)
	msg[3] = 0
	msg[4] = 0 // Identifier
	msg[5] = 1
	msg[6] = 0 // Sequence
	msg[7] = 1
	// Payload: zeros (8 bytes)
	// Compute checksum
	cs := icmpChecksum(msg)
	msg[2] = byte(cs >> 8)
	msg[3] = byte(cs)
	return msg
}

func icmpChecksum(b []byte) uint16 {
	sum := 0
	for i := 0; i < len(b)-1; i += 2 {
		sum += int(b[i])<<8 | int(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += int(b[len(b)-1]) << 8
	}
	for sum>>16 > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// === HTTP Ping (Fallback) ===
func httpPings(ctx context.Context, host string, count int, interval time.Duration) ([]float64, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("https://%s/", host)
	pings := make([]float64, 0, count)

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return pings, ctx.Err()
		default:
		}

		start := time.Now()
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()
		elapsed := time.Since(start)
		pings = append(pings, float64(elapsed.Microseconds())/1000.0)

		if i < count-1 {
			time.Sleep(interval)
		}
	}

	if len(pings) == 0 {
		return nil, fmt.Errorf("no successful HTTP pings to %s", host)
	}
	return pings, nil
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}
