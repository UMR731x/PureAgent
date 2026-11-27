package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// AcquireWorkspace replaces port-based ownership with a Windows file lock.
// The file stays in place; closing the handle releases the lock, including on a crash.
func AcquireWorkspace(directory string) (release func(), running bool, err error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(filepath.Join(directory, ".pureagent.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	var overlapped syscall.Overlapped
	locked, _, err := kernel32.NewProc("LockFileEx").Call(file.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if locked == 0 {
		file.Close()
		if errors.Is(err, syscall.Errno(33)) { // ERROR_LOCK_VIOLATION: another client owns this workspace.
			Activate(directory)
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() { file.Close() }, false, nil
}
