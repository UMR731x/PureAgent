//go:build windows

package desktop

import "syscall"

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	registerClass       = user32.NewProc("RegisterClassExW")
	unregisterClass     = user32.NewProc("UnregisterClassW")
	createWindow        = user32.NewProc("CreateWindowExW")
	defaultWindow       = user32.NewProc("DefWindowProcW")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	postMessage         = user32.NewProc("PostMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	findWindow          = user32.NewProc("FindWindowW")
	createPopupMenu     = user32.NewProc("CreatePopupMenu")
	appendMenu          = user32.NewProc("AppendMenuW")
	destroyMenu         = user32.NewProc("DestroyMenu")
	trackPopupMenu      = user32.NewProc("TrackPopupMenu")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getCursorPosition   = user32.NewProc("GetCursorPos")
	destroyIcon         = user32.NewProc("DestroyIcon")
	messageBox          = user32.NewProc("MessageBoxW")
	openClipboard       = user32.NewProc("OpenClipboard")
	closeClipboard      = user32.NewProc("CloseClipboard")
	emptyClipboard      = user32.NewProc("EmptyClipboard")
	setClipboardData    = user32.NewProc("SetClipboardData")
	globalAlloc         = kernel32.NewProc("GlobalAlloc")
	globalLock          = kernel32.NewProc("GlobalLock")
	globalUnlock        = kernel32.NewProc("GlobalUnlock")
	globalFree          = kernel32.NewProc("GlobalFree")
	moveMemory          = kernel32.NewProc("RtlMoveMemory")
	getModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	registerMessage     = user32.NewProc("RegisterWindowMessageW")
	notifyIcon          = shell32.NewProc("Shell_NotifyIconW")
	extractIcon         = shell32.NewProc("ExtractIconExW")
	shellExecute        = shell32.NewProc("ShellExecuteW")
)

const (
	windowClose   = 0x0010
	windowDestroy = 0x0002
	windowCommand = 0x0111
	trayCallback  = 0x8001
	openWorkspace = 0x8002
	menuWorkspace = 1
	menuDirectory = 2
	menuCopyURL   = 3
	menuExit      = 4
)

type windowClass struct {
	Size, Style                   uint32
	Callback                      uintptr
	ClassExtra, WindowExtra       int32
	Instance, Icon, Cursor, Brush uintptr
	MenuName, ClassName           *uint16
	SmallIcon                     uintptr
}

type point struct{ X, Y int32 }

type windowMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type notificationIcon struct {
	Size             uint32
	Window           uintptr
	ID, Flags        uint32
	Callback         uint32
	Icon             uintptr
	Tip              [128]uint16
	State, StateMask uint32
	Info             [256]uint16
	Version          uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             [16]byte
	BalloonIcon      uintptr
}
