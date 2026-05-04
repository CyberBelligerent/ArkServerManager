//go:build !windows

package server

import "syscall"

func windowsHideAttr() *syscall.SysProcAttr { return nil }
