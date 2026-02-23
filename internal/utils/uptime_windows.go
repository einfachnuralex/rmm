package utils

import (
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getTickCount64 = kernel32.NewProc("GetTickCount64")

// hostUptime returns the host uptime using GetTickCount64
func HostUptime() (float64, error) {
	ret, _, _ := getTickCount64.Call()
	ms := *(*uint64)(unsafe.Pointer(&ret))
	return float64(ms) / 1000.0, nil
}
