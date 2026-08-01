//go:build windows

package engine

import (
	"fmt"
	"syscall"
)

// IP_TTL constant for Windows (same as Linux: 0x4)
const ipTTL = 0x4

func setSocketTTL(fd uintptr, ttl int) error {
	err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, ipTTL, ttl)
	if err != nil {
		return fmt.Errorf("setsockopt IP_TTL: %w", err)
	}
	return nil
}
