//go:build windows

package server

import "syscall"

// windowsHideAttr returns the SysProcAttr that prevents a child
// process from showing a console window.
//
// CREATE_NO_WINDOW = 0x08000000 (Windows process creation flag). The
// child still gets a hidden console it can write to. Capture
// stdout/stderr through the inherited pipes either way.
func windowsHideAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}
