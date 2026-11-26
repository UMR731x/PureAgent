package desktop

import "syscall"

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	setThreadDPI          = user32.NewProc("SetThreadDpiAwarenessContext")
	getWindowDPI          = user32.NewProc("GetDpiForWindow")
	setWindowPosition     = user32.NewProc("SetWindowPos")
	setMenuAppearance     = user32.NewProc("SetMenuInfo")
	fillRectangle         = user32.NewProc("FillRect")
	drawText              = user32.NewProc("DrawTextW")
	drawIcon              = user32.NewProc("DrawIconEx")
	extractSizedIcon      = shell32.NewProc("SHDefExtractIconW")
	createSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	createFont            = gdi32.NewProc("CreateFontW")
	createDrawingPen      = gdi32.NewProc("CreatePen")
	deleteDrawingObject   = gdi32.NewProc("DeleteObject")
	selectDrawingObject   = gdi32.NewProc("SelectObject")
	stockDrawingObject    = gdi32.NewProc("GetStockObject")
	saveDrawingContext    = gdi32.NewProc("SaveDC")
	restoreDrawingContext = gdi32.NewProc("RestoreDC")
	setTextBackground     = gdi32.NewProc("SetBkMode")
	setTextColor          = gdi32.NewProc("SetTextColor")
	roundRectangle        = gdi32.NewProc("RoundRect")
	drawPolyline          = gdi32.NewProc("Polyline")
	drawArc               = gdi32.NewProc("Arc")
)

type rectangle struct{ Left, Top, Right, Bottom int32 }

type menuAppearance struct {
	Size, Mask, Style, MaxHeight uint32
	Background                   uintptr
	HelpID                       uint32
	Data                         uintptr
}

type accessibleMenuText struct {
	Signature, Length uint32
	Text              *uint16
}

type measureMenuItem struct {
	ControlType, ControlID, ID, Width, Height uint32
	Data                                      uintptr
}

type drawMenuItem struct {
	ControlType, ControlID, ID, Action, State uint32
	Window, DC                                uintptr
	Bounds                                    rectangle
	Data                                      uintptr
}
