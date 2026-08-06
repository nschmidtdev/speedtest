package engine

import (
	"context"
	"fmt"
	"net"
	"time"
)

// RunTraceroute führt ein ICMP-basiertes Traceroute zum Ziel aus.
// Gibt bis zu maxHops Stationen zurück.
//
// Benötigt Raw Sockets (root/Admin). Wenn die Berechtigung fehlt,
// wird ein Fehler zurückgegeben, sodass der Aufrufer entscheiden kann,
// ob er das Ergebnis ignorieren oder dem Nutzer eine Meldung zeigen will.
func (e *SpeedtestEngine) RunTraceroute(ctx context.Context, host string, maxHops int, cb ProgressCallback) ([]Hop, error) {
	if maxHops <= 0 {
		maxHops = 30
	}

	if cb != nil {
		cb(ProgressEvent{Type: "progress", Phase: "traceroute", Timestamp: time.Now()})
	}

	// Permission-Check: Ein einzelner traceHop-Versuch zeigt ob Raw Sockets verfügbar sind.
	probe, _, probeErr := traceHop(ctx, net.IPv4(127, 0, 0, 1), 1)
	if probeErr != nil {
		return nil, fmt.Errorf("traceroute requires raw socket privileges (root/admin): %w", probeErr)
	}
	_ = probe

	// Resolve target IP
	targetIP, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", host, err)
	}

	hops := make([]Hop, 0, maxHops)

	for ttl := 1; ttl <= maxHops; ttl++ {
		select {
		case <-ctx.Done():
			return hops, ctx.Err()
		default:
		}

		hop, final, err := traceHop(ctx, targetIP.IP, ttl)
		if err != nil {
			hops = append(hops, Hop{TTL: ttl, Address: "*", RttMs: 0})
			continue
		}

		hops = append(hops, hop)

		if cb != nil {
			cb(ProgressEvent{
				Type:      "progress",
				Phase:     "traceroute",
				Timestamp: time.Now(),
			})
		}

		if final {
			break
		}
	}

	return hops, nil
}

// traceHop sends one ICMP packet with given TTL and waits for response.
// Returns the hop info, whether we've reached the target, and any error.
func traceHop(ctx context.Context, targetIP net.IP, ttl int) (Hop, bool, error) {
	// Open a raw ICMP listener
	conn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return Hop{}, false, fmt.Errorf("icmp listen: %w", err)
	}
	defer conn.Close()

	// Open a raw connection to the target with specific TTL
	rawConn, err := net.DialIP("ip4:icmp", nil, &net.IPAddr{IP: targetIP})
	if err != nil {
		return Hop{}, false, fmt.Errorf("dial: %w", err)
	}
	defer rawConn.Close()

	// Set TTL on the raw connection
	if err := setTTL(rawConn, ttl); err != nil {
		return Hop{}, false, fmt.Errorf("set ttl: %w", err)
	}

	// Build ICMP Echo Request
	msg := makeICMPEcho()

	// Send 3 probes, take best RTT
	bestRTT := time.Duration(0)
	hopAddr := ""
	var reachedTarget bool

	for probe := 0; probe < 3; probe++ {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		rawConn.SetWriteDeadline(time.Now().Add(2 * time.Second))

		start := time.Now()
		_, err := rawConn.Write(msg)
		if err != nil {
			continue
		}

		resp := make([]byte, 1500)
		n, src, err := conn.ReadFrom(resp)
		if err != nil {
			continue
		}

		elapsed := time.Since(start)

		// Parse ICMP response
		if n < 8 {
			continue
		}

		icmpType := resp[0]
		srcIP := src.String()

		// Time Exceeded (type 11) = intermediate hop
		// Echo Reply (type 0) = reached target
		if icmpType == 11 || icmpType == 0 {
			hopAddr = srcIP
			if elapsed > bestRTT {
				bestRTT = elapsed
			}
			if icmpType == 0 {
				reachedTarget = true
			}
		}
	}

	if hopAddr == "" {
		return Hop{TTL: ttl, Address: "*"}, false, nil
	}

	hop := Hop{
		TTL:     ttl,
		Address: hopAddr,
		RttMs:   float64(bestRTT.Microseconds()) / 1000.0,
	}

	// Try reverse DNS
	names, err := net.LookupAddr(hopAddr)
	if err == nil && len(names) > 0 {
		hop.Host = names[0]
	}

	return hop, reachedTarget, nil
}

// setTTL sets the IP TTL on a raw connection.
// On Windows, this requires IP_TTL via socket option.
func setTTL(conn *net.IPConn, ttl int) error {
	// Use raw syscalls — on Windows, we need IP_TTL (0x4)
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		sockErr = setSocketTTL(fd, ttl)
	})
	if err != nil {
		return err
	}
	return sockErr
}

// makeICMPEcho is already defined in bufferbloat.go
// icmpChecksum is already defined in bufferbloat.go
