//go:build !windows

package main

import (
	"os"
	"syscall"
)

func setUidGidFromFile(path string) error {
	// Find out the owner of the given path
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	statt := stat.Sys().(*syscall.Stat_t)
	syscall.Setgid(int(statt.Gid))
	return syscall.Setuid(int(statt.Uid))
}
