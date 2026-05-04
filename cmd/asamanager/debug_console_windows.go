//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachDebugConsole calls AllocConsole and rebinds os.Stdout/os.Stderr
// to the new console. Used when -debug is passed on the command line
func attachDebugConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole := kernel32.NewProc("AllocConsole")
	if r1, _, _ := procAllocConsole.Call(); r1 == 0 {
		return
	}
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil && h != 0 {
		os.Stdout = os.NewFile(uintptr(h), "stdout")
	}
	if h, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE); err == nil && h != 0 {
		os.Stderr = os.NewFile(uintptr(h), "stderr")
	}
}
