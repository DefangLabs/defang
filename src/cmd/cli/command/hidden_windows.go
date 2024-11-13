package command

import (
	"os"
	"syscall"
)

func WriteHiddenFile(path string, data []byte, perm os.FileMode) error {
	err := os.WriteFile(path, data, perm)
	if err != nil {
		return err
	}

	winPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	err = syscall.SetFileAttributes(winPath, syscall.FILE_ATTRIBUTE_HIDDEN)
	if err != nil {
		return err
	}

	return nil
}

func ReadHiddenFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
