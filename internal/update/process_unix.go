//go:build !windows

package update

import (
	"io"
	"os"
	"os/exec"
	"syscall"
)

func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func waitForParent(pid int) (func() error, error) {
	return func() error {
		_, err := io.Copy(io.Discard, os.Stdin)
		return err
	}, nil
}
