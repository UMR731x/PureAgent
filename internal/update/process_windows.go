package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

func detach(command *exec.Cmd) {
	// Detach from the console without hiding the desktop client's first window.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
}

func waitForParent(pid int) (func() error, error) {
	process, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	return func() error {
		defer syscall.CloseHandle(process)
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			return err
		}
		status, err := syscall.WaitForSingleObject(process, syscall.INFINITE)
		if err != nil {
			return err
		}
		if status != syscall.WAIT_OBJECT_0 {
			return fmt.Errorf("等待旧进程退出失败：%d", status)
		}
		return nil
	}, nil
}
