package update

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

// A detached GUI client must be able to show its first window after an update.
func TestDetachedClientCanShowWindow(t *testing.T) {
	if os.Getenv("PUREAGENT_TEST_DETACHED_WINDOW") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(executable, "-test.run=^TestDetachedClientCanShowWindow$")
		command.Env = append(os.Environ(), "PUREAGENT_TEST_DETACHED_WINDOW=1")
		detach(command)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("detached desktop startup: %v\n%s", err, output)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := syscall.NewLazyDLL("user32.dll")
	class, _ := syscall.UTF16PtrFromString("STATIC")
	window, _, err := user32.NewProc("CreateWindowExW").Call(
		0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 32, 32, 0, 0, 0, 0,
	)
	if window == 0 {
		t.Fatalf("create desktop window: %v", err)
	}
	defer user32.NewProc("DestroyWindow").Call(window)
	user32.NewProc("ShowWindow").Call(window, 5) // SW_SHOW, as used by the desktop window.
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(window)
	if visible == 0 {
		t.Fatal("the update launch hid the desktop client's first window")
	}
}
