package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// hostUptime reads the host uptime from /proc/uptime
func HostUptime() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/uptime: %w", err)
	}
	// Format: "12345.67 23456.78" – first field is uptime in seconds
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	return strconv.ParseFloat(fields[0], 64)
}
