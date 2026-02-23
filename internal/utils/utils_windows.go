package utils

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getTickCount64 = kernel32.NewProc("GetTickCount64")

	pdh                         = syscall.NewLazyDLL("pdh.dll")
	pdhOpenQuery                = pdh.NewProc("PdhOpenQuery")
	pdhAddEnglishCounterW       = pdh.NewProc("PdhAddEnglishCounterW")
	pdhCollectQueryData         = pdh.NewProc("PdhCollectQueryData")
	pdhGetFormattedCounterValue = pdh.NewProc("PdhGetFormattedCounterValue")
	pdhCloseQuery               = pdh.NewProc("PdhCloseQuery")
)

const pdhFmtDouble = 0x00000200

type pdhFmtCounterValue struct {
	status uint32
	value  float64
}

// HostUptime returns the host uptime using GetTickCount64
func HostUptime() (float64, error) {
	ret, _, _ := getTickCount64.Call()
	ms := *(*uint64)(unsafe.Pointer(&ret))
	return float64(ms) / 1000.0, nil
}

// HostLoad5 approximates load by querying CPU utilisation via PDH.
// Windows has no native load average; this returns the current processor
// time percentage as a reasonable substitute.
func HostLoad5() (float64, error) {
	var query uintptr
	ret, _, _ := pdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query)))
	if ret != 0 {
		return 0, fmt.Errorf("PdhOpenQuery failed: 0x%x", ret)
	}
	defer pdhCloseQuery.Call(query)

	counter := `\Processor(_Total)\% Processor Time`
	counterUTF16, _ := syscall.UTF16PtrFromString(counter)

	var hCounter uintptr
	ret, _, _ = pdhAddEnglishCounterW.Call(query, uintptr(unsafe.Pointer(counterUTF16)), 0, uintptr(unsafe.Pointer(&hCounter)))
	if ret != 0 {
		return 0, fmt.Errorf("PdhAddEnglishCounterW failed: 0x%x", ret)
	}

	// PDH requires two collections to compute a rate
	pdhCollectQueryData.Call(query)
	pdhCollectQueryData.Call(query)

	var val pdhFmtCounterValue
	ret, _, _ = pdhGetFormattedCounterValue.Call(hCounter, pdhFmtDouble, 0, uintptr(unsafe.Pointer(&val)))
	if ret != 0 {
		return 0, fmt.Errorf("PdhGetFormattedCounterValue failed: 0x%x", ret)
	}
	return val.value, nil
}
