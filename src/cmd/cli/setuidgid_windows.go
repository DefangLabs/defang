//go:build windows

package main

func setUidGidFromFile(path string) error {
	// Windows does not support changing file ownership
	return nil
}
