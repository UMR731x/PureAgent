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
	options      Options
	window       uintptr
	icon         notificationIcon
	taskbarReady uint32
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

func Activate(address string) bool {
	window, _, _ := findWindow.Call(uintptr(unsafe.Pointer(wide(trayClass))), uintptr(unsafe.Pointer(wide(address))))
	if window == 0 {
		return false
	}
	postMessage.Call(window, openWorkspace, 0, 0)
	return true
}

func Start(options Options) (func(), error) {
	tray := &trayWindow{options: options}
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
	instance, _, _ := getModuleHandle.Call(0)
	class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Instance: instance,
		ClassName: wide(trayClass), Callback: syscall.NewCallback(tray.dispatch)}
	if registered, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); registered == 0 {
		ready <- fmt.Errorf("注册托盘窗口失败：%w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(class.ClassName)), instance)
	window, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class.ClassName)),
		uintptr(unsafe.Pointer(wide(tray.options.URL))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
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
	if tray.options.OpenBrowser {
		tray.open(tray.options.URL)
	}
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
		tray.open(tray.options.URL)
	case windowCommand:
		tray.command(wParam & 0xffff)
	case trayCallback:
		switch lParam & 0xffff {
		case 0x0400, 0x0401: // NIN_SELECT, NIN_KEYSELECT
			tray.open(tray.options.URL)
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

func (tray *trayWindow) showMenu() {
	menu, _, _ := createPopupMenu.Call()
	defer destroyMenu.Call(menu)
	for _, item := range []struct {
		id    uintptr
		label string
	}{{menuWorkspace, "打开工作台"}, {menuDirectory, "打开数据目录"}, {menuCopyURL, "复制访问地址"}} {
		appendMenu.Call(menu, 0, item.id, uintptr(unsafe.Pointer(wide(item.label))))
	}
	appendMenu.Call(menu, 0x800, 0, 0) // MF_SEPARATOR
	appendMenu.Call(menu, 0, menuExit, uintptr(unsafe.Pointer(wide("退出 PureAgent"))))
	var cursor point
	getCursorPosition.Call(uintptr(unsafe.Pointer(&cursor)))
	setForegroundWindow.Call(tray.window)
	selected, _, _ := trackPopupMenu.Call(menu, 0x0102, uintptr(cursor.X), uintptr(cursor.Y), 0, tray.window, 0)
	postMessage.Call(tray.window, 0, 0, 0) // Complete the notification-area menu interaction.
	tray.command(selected)
}

func (tray *trayWindow) command(command uintptr) {
	switch command {
	case menuWorkspace:
		tray.open(tray.options.URL)
	case menuDirectory:
		tray.open(tray.options.Directory)
	case menuCopyURL:
		if err := tray.copyURL(); err != nil {
			ReportError(err)
		}
	case menuExit:
		tray.options.Exit()
	}
}

func (tray *trayWindow) open(target string) {
	result, _, _ := shellExecute.Call(tray.window, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(target))), 0, 0, 1)
	if result <= 32 {
		ReportError(fmt.Errorf("无法打开 %s（Windows 错误 %d）", target, result))
	}
}

func (tray *trayWindow) copyURL() error {
	if ok, _, err := openClipboard.Call(tray.window); ok == 0 {
		return fmt.Errorf("无法打开剪贴板：%w", err)
	}
	defer closeClipboard.Call()
	text := syscall.StringToUTF16(tray.options.URL)
	memory, _, err := globalAlloc.Call(2, uintptr(len(text)*2))
	if memory == 0 {
		return err
	}
	address, _, err := globalLock.Call(memory)
	if address == 0 {
		globalFree.Call(memory)
		return err
	}
	moveMemory.Call(address, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)*2))
	globalUnlock.Call(memory)
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		globalFree.Call(memory)
		return err
	}
	if ok, _, err := setClipboardData.Call(13, memory); ok == 0 { // CF_UNICODETEXT; ownership passes to Windows.
		globalFree.Call(memory)
		return err
	}
	return nil
}

func ReportError(err error) {
	messageBox.Call(0, uintptr(unsafe.Pointer(wide(err.Error()))), uintptr(unsafe.Pointer(wide("PureAgent"))), 0x10)
}
