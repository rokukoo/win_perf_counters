//go:build windows

package win_perf_counters

import (
	"syscall"
)

type fileTime struct {
	dwLowDateTime  uint32
	dwHighDateTime uint32
}

var (
	// Library
	libKernelDll *syscall.DLL

	// Functions
	kernelLocalFileTimeToFileTime *syscall.Proc
)

func init() {
	libKernelDll = syscall.MustLoadDLL("Kernel32.dll")

	kernelLocalFileTimeToFileTime = libKernelDll.MustFindProc("LocalFileTimeToFileTime")
}