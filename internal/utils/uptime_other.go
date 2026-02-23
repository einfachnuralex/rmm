//go:build !linux && !windows && !darwin

package utils

import "fmt"

// hostUptime is not implemented for this platform
func HostUptime() (float64, error) {
	return 0, fmt.Errorf("host uptime not supported on this platform")
}
