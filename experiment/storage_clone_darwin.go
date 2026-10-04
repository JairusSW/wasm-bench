package experiment

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func cloneExclusive(src, dst string) (bool, error) {
	err := unix.Clonefile(src, dst, 0)
	if err == nil {
		return true, os.Chmod(dst, 0644)
	}
	// Different filesystems and filesystems without cloning use ordinary copies.
	if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return false, nil
	}
	return false, &os.PathError{Op: "clonefile", Path: dst, Err: err}
}
