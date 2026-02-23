//go:build !linux && !windows && !darwin

package utils

import "fmt"

// HostUptime is not implemented for this platform
func HostUptime() (float64, error) {
	return 0, fmt.Errorf("host uptime not supported on this platform")
}

// HostLoad5 is not implemented for this platform
func HostLoad5() (float64, error) {
	return 0, fmt.Errorf("host load not supported on this platform")
}
