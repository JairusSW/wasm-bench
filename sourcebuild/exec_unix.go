//go:build linux || darwin

package sourcebuild

import (
	"os/exec"
	"syscall"
)

func supported() error { return nil }
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
