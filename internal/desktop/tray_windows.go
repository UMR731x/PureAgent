package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const trayClass = "PureAgent.Tray"

type trayWindow struct {
	options       Options
	window        uintptr
	icon          notificationIcon
	taskbarReady  uint32
	menu          *trayMenu
	openWorkspace func()
	exit          func()
}

func wide(value string) *uint16 { return syscall.StringToUTF16Ptr(value) }

func DefaultDataDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(executable)
	if strings.EqualFold(filepath.Base(directory), "bin") {
		directory = filepath.Dir(directory)
	}
	return filepath.Join(directory, "data"), nil
}

func Activate(directory string) bool {
	window, _, _ := findWindow.Call(uintptr(unsafe.Pointer(wide(trayClass))), uintptr(unsafe.Pointer(wide(directory))))
	if window == 0 {
		return false
	}
	postMessage.Call(window, openWorkspace, 0, 0)
	return true
}

func startTray(options Options, openWorkspace, exit func()) (func(), error) {
	tray := &trayWindow{options: options, openWorkspace: openWorkspace, exit: exit}
	ready := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		tray.run(ready)
	}()
	if err := <-ready; err != nil {
		<-done
		return nil, err
	}
	return func() {
		postMessage.Call(tray.window, windowClose, 0, 0)
		<-done
	}, nil
}

func (tray *trayWindow) run(ready chan<- error) {
	// Keep menu text and line icons sharp when the taskbar is on a scaled display.
	previousDPI, _, _ := setThreadDPI.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	defer setThreadDPI.Call(previousDPI)
	instance, _, _ := getModuleHandle.Call(0)
	class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Instance: instance,
		ClassName: wide(trayClass), Callback: syscall.NewCallback(tray.dispatch)}
	if registered, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); registered == 0 {
		ready <- fmt.Errorf("注册托盘窗口失败：%w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(class.ClassName)), instance)
	window, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class.ClassName)),
		uintptr(unsafe.Pointer(wide(tray.options.Directory))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		ready <- fmt.Errorf("创建托盘窗口失败：%w", err)
		return
	}
	tray.window = window
	if err := tray.loadIcon(); err != nil {
		user32.NewProc("DestroyWindow").Call(window)
		ready <- err
		return
	}
	defer destroyIcon.Call(tray.icon.Icon)
	message, _, _ := registerMessage.Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	tray.taskbarReady = uint32(message)
	ready <- nil
	var event windowMessage
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&event)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&event)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&event)))
	}
}

func (tray *trayWindow) loadIcon() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	tray.icon = notificationIcon{Size: uint32(unsafe.Sizeof(notificationIcon{})), Window: tray.window,
		ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: trayCallback}
	count, _, err := extractIcon.Call(uintptr(unsafe.Pointer(wide(executable))), 0, 0, uintptr(unsafe.Pointer(&tray.icon.Icon)), 1)
	if count == 0 || uint32(count) == ^uint32(0) {
		return fmt.Errorf("读取 PureAgent 图标失败：%w", err)
	}
	copy(tray.icon.Tip[:], syscall.StringToUTF16("PureAgent "+tray.options.Version+" · 点击打开工作台"))
	if ok, _, err := notifyIcon.Call(0, uintptr(unsafe.Pointer(&tray.icon))); ok == 0 {
		destroyIcon.Call(tray.icon.Icon)
		return fmt.Errorf("创建托盘图标失败：%w", err)
	}
	tray.icon.Version = 4
	notifyIcon.Call(4, uintptr(unsafe.Pointer(&tray.icon)))
	return nil
}

func (tray *trayWindow) dispatch(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case openWorkspace:
		tray.openWorkspace()
	case windowCommand:
		tray.command(wParam & 0xffff)
	case 0x002c: // WM_MEASUREITEM
		if tray.menu != nil {
			tray.menu.measure(lParam)
			return 1
		}
	case 0x002b: // WM_DRAWITEM
		if tray.menu != nil {
			tray.menu.draw(lParam)
			return 1
		}
	case trayCallback:
		switch lParam & 0xffff {
		case 0x0400, 0x0401: // NIN_SELECT, NIN_KEYSELECT
			tray.openWorkspace()
		case 0x007b: // WM_CONTEXTMENU
			tray.showMenu()
		}
	case windowDestroy:
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&tray.icon)))
		postQuitMessage.Call(0)
	default:
		if tray.taskbarReady != 0 && message == tray.taskbarReady {
			notifyIcon.Call(0, uintptr(unsafe.Pointer(&tray.icon)))
			notifyIcon.Call(4, uintptr(unsafe.Pointer(&tray.icon)))
		} else {
			result, _, _ := defaultWindow.Call(window, uintptr(message), wParam, lParam)
			return result
		}
	}
	return 0
}

func (tray *trayWindow) command(command uintptr) {
	switch command {
	case menuWorkspace:
		tray.openWorkspace()
	case menuDirectory:
		tray.open(tray.options.Directory)
	case menuExit:
		tray.exit()
	}
}

func (tray *trayWindow) open(target string) {
	result, _, _ := shellExecute.Call(tray.window, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(target))), 0, 0, 1)
	if result <= 32 {
		ReportError(fmt.Errorf("无法打开 %s（Windows 错误 %d）", target, result))
	}
}

func ReportError(err error) {
	messageBox.Call(0, uintptr(unsafe.Pointer(wide(err.Error()))), uintptr(unsafe.Pointer(wide("PureAgent"))), 0x10)
}
