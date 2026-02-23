package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// HostUptime reads the host uptime from /proc/uptime
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

// HostLoad5 returns the 5-minute load average from /proc/loadavg
func HostLoad5() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/loadavg: %w", err)
	}
	// Format: "0.10 0.25 0.30 1/123 456" – second field is 5-min average
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/loadavg format")
	}
	return strconv.ParseFloat(fields[1], 64)
}
