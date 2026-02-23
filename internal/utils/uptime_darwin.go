package utils

import (
	"syscall"
	"time"
	"unsafe"
)

// hostUptime returns the host uptime via sysctl kern.boottime
func HostUptime() (float64, error) {
	// kern.boottime returns a struct timeval with the boot time
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
