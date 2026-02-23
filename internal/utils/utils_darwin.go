package utils

import (
	"syscall"
	"time"
	"unsafe"
)

// loadavg mirrors the C struct loadavg from <sys/resource.h>
type loadavg struct {
	load  [3]uint32
	scale uint32
}

// HostUptime returns the host uptime via sysctl kern.boottime
func HostUptime() (float64, error) {
	tv := syscall.Timeval{}
	mib := []int32{1, 21} // CTL_KERN=1, KERN_BOOTTIME=21
	size := uintptr(unsafe.Sizeof(tv))

	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		2,
		uintptr(unsafe.Pointer(&tv)),
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)
	if errno != 0 {
		return 0, errno
	}
	bootTime := time.Unix(tv.Sec, int64(tv.Usec)*1000)
	return time.Since(bootTime).Seconds(), nil
}

// HostLoad5 returns the 5-minute load average via sysctl vm.loadavg
func HostLoad5() (float64, error) {
	var avg loadavg
	mib := []int32{2, 2} // CTL_VM=2, VM_LOADAVG=2
	size := uintptr(unsafe.Sizeof(avg))

	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		2,
		uintptr(unsafe.Pointer(&avg)),
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)
	if errno != 0 {
		return 0, errno
	}
	return float64(avg.load[1]) / float64(avg.scale), nil
}
