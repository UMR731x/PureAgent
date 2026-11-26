package desktop

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	menuHeading = 100
	menuDivider = 101
	menuBottom  = 102
	menuSurface = 0xf9faf7
	menuInk     = 0x29332b
	menuGreen   = 0x4a6240
	menuMuted   = 0x849080
	menuRed     = 0xaf5348
)

type menuEntry struct {
	accessible accessibleMenuText
	id         uintptr
	label      string
	text       []uint16
}

type trayMenu struct {
	handle, background, font, headingFont, captionFont, icon uintptr
	dpi                                                      int32
	entries                                                  []menuEntry
}

func (tray *trayWindow) showMenu() {
	var cursor point
	getCursorPosition.Call(uintptr(unsafe.Pointer(&cursor)))
	// Move only the hidden owner so GetDpiForWindow reflects the menu's monitor.
	setWindowPosition.Call(tray.window, 0, uintptr(cursor.X), uintptr(cursor.Y), 0, 0, 0x0015)
	dpi, _, _ := getWindowDPI.Call(tray.window)
	menu := newTrayMenu(int32(dpi))
	tray.menu = menu
	setForegroundWindow.Call(tray.window)
	selected, _, _ := trackPopupMenu.Call(menu.handle, 0x0102, uintptr(cursor.X), uintptr(cursor.Y), 0, tray.window, 0)
	tray.menu = nil
	menu.close()
	runtime.KeepAlive(menu) // Windows borrows each entry's accessible name while tracking the menu.
	postMessage.Call(tray.window, 0, 0, 0)
	tray.command(selected)
}

func newTrayMenu(dpi int32) *trayMenu {
	menu := &trayMenu{dpi: dpi, entries: []menuEntry{
		{id: menuHeading, label: "PureAgent"},
		{id: menuWorkspace, label: "打开工作台"},
		{id: menuDirectory, label: "打开数据目录"},
		{id: menuCopyURL, label: "复制访问地址"},
		{id: menuDivider},
		{id: menuExit, label: "退出 PureAgent"},
		{id: menuBottom},
	}}
	menu.handle, _, _ = createPopupMenu.Call()
	menu.background, _, _ = createSolidBrush.Call(colorRef(menuSurface))
	menu.font = menu.createFont("Microsoft YaHei UI", 16, 400)
	menu.headingFont = menu.createFont("Segoe UI", 17, 600)
	menu.captionFont = menu.createFont("Microsoft YaHei UI", 12, 400)
	executable, _ := os.Executable()
	extractSizedIcon.Call(uintptr(unsafe.Pointer(wide(executable))), 0, 0, uintptr(unsafe.Pointer(&menu.icon)), 0, uintptr(menu.pixels(32)))
	appearance := menuAppearance{Size: uint32(unsafe.Sizeof(menuAppearance{})), Mask: 0x12, Style: 0x80000000, Background: menu.background}
	setMenuAppearance.Call(menu.handle, uintptr(unsafe.Pointer(&appearance))) // MNS_NOCHECK: no classic checkmark gutter.
	for i := range menu.entries {
		entry := &menu.entries[i]
		entry.text = syscall.StringToUTF16(entry.label)
		entry.accessible = accessibleMenuText{Signature: 0xaa0df00d, Length: uint32(len(entry.text) - 1), Text: &entry.text[0]}
		flags := uintptr(0x100) // MF_OWNERDRAW
		if entry.id >= menuHeading {
			flags |= 3 // Decorative rows cannot be focused or activated.
		}
		appendMenu.Call(menu.handle, flags, entry.id, uintptr(unsafe.Pointer(&entry.accessible)))
	}
	return menu
}

func (menu *trayMenu) close() {
	destroyMenu.Call(menu.handle)
	for _, resource := range []uintptr{menu.background, menu.font, menu.headingFont, menu.captionFont} {
		deleteDrawingObject.Call(resource)
	}
	destroyIcon.Call(menu.icon)
}

func (menu *trayMenu) pixels(value int32) int32 { return (value*menu.dpi + 48) / 96 }

func (menu *trayMenu) createFont(face string, size, weight int32) uintptr {
	font, _, _ := createFont.Call(uintptr(-menu.pixels(size)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide(face))))
	return font
}

func (menu *trayMenu) measure(address uintptr) {
	var item measureMenuItem
	moveMemory.Call(uintptr(unsafe.Pointer(&item)), address, unsafe.Sizeof(item))
	height := int32(44)
	switch item.ID {
	case menuHeading:
		height = 68
	case menuDivider:
		height = 12
	case menuBottom:
		height = 6
	}
	item.Width, item.Height = uint32(menu.pixels(264)), uint32(menu.pixels(height))
	moveMemory.Call(address, uintptr(unsafe.Pointer(&item)), unsafe.Sizeof(item))
}

func (menu *trayMenu) draw(address uintptr) {
	var item drawMenuItem
	moveMemory.Call(uintptr(unsafe.Pointer(&item)), address, unsafe.Sizeof(item))
	saved, _, _ := saveDrawingContext.Call(item.DC)
	defer restoreDrawingContext.Call(item.DC, saved)
	fillRectangle.Call(item.DC, uintptr(unsafe.Pointer(&item.Bounds)), menu.background)
	setTextBackground.Call(item.DC, 1) // TRANSPARENT
	switch item.ID {
	case menuHeading:
		menu.drawHeading(item.DC, item.Bounds)
	case menuDivider:
		line := item.Bounds
		line.Left += menu.pixels(16)
		line.Right -= menu.pixels(16)
		line.Top += menu.pixels(6)
		line.Bottom = line.Top + menu.pixels(1)
		fillMenuShape(item.DC, line, 0xe4e8df, 0)
	case menuBottom:
		return
	default:
		menu.drawCommand(item)
	}
}

func (menu *trayMenu) drawHeading(dc uintptr, bounds rectangle) {
	left, top := bounds.Left+menu.pixels(17), bounds.Top+menu.pixels(16)
	drawIcon.Call(dc, uintptr(left), uintptr(top), menu.icon, uintptr(menu.pixels(32)), uintptr(menu.pixels(32)), 0, 0, 3)
	title := rectangle{bounds.Left + menu.pixels(62), bounds.Top + menu.pixels(10), bounds.Right - menu.pixels(16), bounds.Top + menu.pixels(35)}
	drawMenuText(dc, menu.headingFont, "PureAgent", title, menuInk)
	caption := title
	caption.Top, caption.Bottom = bounds.Top+menu.pixels(35), bounds.Top+menu.pixels(55)
	drawMenuText(dc, menu.captionFont, "本地图像工作台", caption, menuMuted)
	line := rectangle{bounds.Left + menu.pixels(16), bounds.Bottom - menu.pixels(5), bounds.Right - menu.pixels(16), bounds.Bottom - menu.pixels(4)}
	fillMenuShape(dc, line, 0xe4e8df, 0)
}

func (menu *trayMenu) drawCommand(item drawMenuItem) {
	ink, glyph := uint32(menuInk), uint32(0x73856b)
	if item.ID == menuExit {
		ink, glyph = menuRed, menuRed
	}
	if item.State&1 != 0 { // ODS_SELECTED covers both mouse hover and keyboard focus.
		background := uint32(0xe8efdf)
		if item.ID == menuExit {
			background = 0xf7eae4
		} else {
			ink, glyph = menuGreen, menuGreen
		}
		bounds := item.Bounds
		bounds.Left += menu.pixels(7)
		bounds.Right -= menu.pixels(7)
		bounds.Top += menu.pixels(3)
		bounds.Bottom -= menu.pixels(3)
		fillMenuShape(item.DC, bounds, background, menu.pixels(8))
	}
	menu.drawGlyph(item.DC, item.ID, item.Bounds.Left+menu.pixels(19), item.Bounds.Top+menu.pixels(11), glyph)
	bounds := item.Bounds
	bounds.Left += menu.pixels(56)
	bounds.Right -= menu.pixels(16)
	for _, entry := range menu.entries {
		if uint32(entry.id) == item.ID {
			drawMenuText(item.DC, menu.font, entry.label, bounds, ink)
			break
		}
	}
}

func colorRef(rgb uint32) uintptr { return uintptr((rgb&0xff)<<16 | rgb&0xff00 | rgb>>16) }

func drawMenuText(dc, font uintptr, text string, bounds rectangle, color uint32) {
	selectDrawingObject.Call(dc, font)
	setTextColor.Call(dc, colorRef(color))
	drawText.Call(dc, uintptr(unsafe.Pointer(wide(text))), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), 0x824)
}

func fillMenuShape(dc uintptr, bounds rectangle, color uint32, radius int32) {
	brush, _, _ := createSolidBrush.Call(colorRef(color))
	if radius == 0 {
		fillRectangle.Call(dc, uintptr(unsafe.Pointer(&bounds)), brush)
	} else {
		oldBrush, _, _ := selectDrawingObject.Call(dc, brush)
		noPen, _, _ := stockDrawingObject.Call(8) // NULL_PEN
		oldPen, _, _ := selectDrawingObject.Call(dc, noPen)
		roundRectangle.Call(dc, uintptr(bounds.Left), uintptr(bounds.Top), uintptr(bounds.Right), uintptr(bounds.Bottom), uintptr(radius*2), uintptr(radius*2))
		selectDrawingObject.Call(dc, oldPen)
		selectDrawingObject.Call(dc, oldBrush)
	}
	deleteDrawingObject.Call(brush)
}

func (menu *trayMenu) drawGlyph(dc uintptr, command uint32, left, top int32, color uint32) {
	pen, _, _ := createDrawingPen.Call(0, uintptr(menu.pixels(2)), colorRef(color))
	oldPen, _, _ := selectDrawingObject.Call(dc, pen)
	noBrush, _, _ := stockDrawingObject.Call(5) // NULL_BRUSH
	oldBrush, _, _ := selectDrawingObject.Call(dc, noBrush)
	stroke := func(points ...point) {
		for i := range points {
			points[i].X = left + menu.pixels(points[i].X)
			points[i].Y = top + menu.pixels(points[i].Y)
		}
		drawPolyline.Call(dc, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points)))
	}
	box := func(x1, y1, x2, y2 int32) {
		roundRectangle.Call(dc, uintptr(left+menu.pixels(x1)), uintptr(top+menu.pixels(y1)), uintptr(left+menu.pixels(x2)), uintptr(top+menu.pixels(y2)), uintptr(menu.pixels(4)), uintptr(menu.pixels(4)))
	}
	switch command {
	case menuWorkspace:
		box(2, 3, 22, 20)
		stroke(point{2, 8}, point{22, 8})
	case menuDirectory:
		stroke(point{3, 8}, point{3, 5}, point{9, 5}, point{12, 8}, point{21, 8}, point{21, 20}, point{3, 20}, point{3, 8})
	case menuCopyURL:
		box(8, 8, 21, 21)
		stroke(point{15, 8}, point{15, 3}, point{3, 3}, point{3, 15}, point{8, 15})
	case menuExit:
		drawArc.Call(dc, uintptr(left+menu.pixels(4)), uintptr(top+menu.pixels(4)), uintptr(left+menu.pixels(21)), uintptr(top+menu.pixels(21)),
			uintptr(left+menu.pixels(8)), uintptr(top+menu.pixels(5)), uintptr(left+menu.pixels(17)), uintptr(top+menu.pixels(5)))
		stroke(point{12, 2}, point{12, 12})
	}
	selectDrawingObject.Call(dc, oldBrush)
	selectDrawingObject.Call(dc, oldPen)
	deleteDrawingObject.Call(pen)
}
