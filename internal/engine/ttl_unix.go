//go:build !windows

package engine

import (
	"fmt"
	"syscall"
)

const ipTTL = 0x4 // IP_TTL

func setSocketTTL(fd uintptr, ttl int) error {
	err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipTTL, ttl)
	if err != nil {
		return fmt.Errorf("setsockopt IP_TTL: %w", err)
	}
	return nil
}
