//go:build windows

package main

import (
	"image"
	"math"
	"syscall"
	"unsafe"

	"chibipet/internal/pet"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procSetTimer                      = user32.NewProc("SetTimer")
	procKillTimer                     = user32.NewProc("KillTimer")
	procUpdateLayeredWindow           = user32.NewProc("UpdateLayeredWindow")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procSetCapture                    = user32.NewProc("SetCapture")
	procReleaseCapture                = user32.NewProc("ReleaseCapture")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procLoadImageW                    = user32.NewProc("LoadImageW")
	procSetCursor                     = user32.NewProc("SetCursor")
	procSystemParametersInfoW         = user32.NewProc("SystemParametersInfoW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procEnumWindows                   = user32.NewProc("EnumWindows")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsIconic                      = user32.NewProc("IsIconic")
	procGetWindowLongW                = user32.NewProc("GetWindowLongW")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procGetClassNameW                 = user32.NewProc("GetClassNameW")
	procGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")

	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wsPopup   = 0x80000000
	wsVisible = 0x10000000

	wsExTopmost     = 0x00000008
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000

	gwlExStyle               = ^uintptr(19) // -20
	dwmwaExtendedFrameBounds = 9
	dwmwaCloaked             = 14

	hwndTopmost    = ^uintptr(0) // (HWND)-1
	hwndNoTopmost  = ^uintptr(1) // (HWND)-2
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
	swpNoActivate  = 0x0010

	wmNull           = 0x0000
	wmDestroy        = 0x0002
	wmSettingChange  = 0x001A
	wmSetCursor      = 0x0020
	wmDisplayChange  = 0x007E
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmCaptureChanged = 0x0215
	wmApp            = 0x8000
	wmTrayIcon       = wmApp + 1

	ulwAlpha    = 0x00000002
	acSrcOver   = 0x00
	acSrcAlpha  = 0x01
	biRGB       = 0
	dibRGBColor = 0

	spiGetWorkArea = 0x0030
	smCXScreen     = 0
	smCYScreen     = 1

	idcArrow   = 32512
	idcSizeAll = 32646
	idcHand    = 32649

	mfString    = 0x00000000
	mfGrayed    = 0x00000001
	mfChecked   = 0x00000008
	mfPopup     = 0x00000010
	mfSeparator = 0x00000800

	tpmRightButton  = 0x0002
	tpmReturnCmd    = 0x0100
	tpmNonotify     = 0x0080
	tpmBottomAlign  = 0x0020

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mbOK              = 0x00000000
	mbIconInformation = 0x00000040

	errorAlreadyExists = 183

	dpiAwarenessPerMonitorAwareV2 = ^uintptr(3) // (DPI_AWARENESS_CONTEXT)-4
)

type point struct {
	X, Y int32
}

type sizeStruct struct {
	CX, CY int32
}

type rectStruct struct {
	Left, Top, Right, Bottom int32
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msgStruct struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// Compile-time ABI assertions verifying exact Windows amd64 Win32 struct sizes.
const (
	_ uint = 8 - uint(unsafe.Sizeof(point{}))
	_ uint = uint(unsafe.Sizeof(point{})) - 8
	_ uint = 8 - uint(unsafe.Sizeof(sizeStruct{}))
	_ uint = uint(unsafe.Sizeof(sizeStruct{})) - 8
	_ uint = 16 - uint(unsafe.Sizeof(rectStruct{}))
	_ uint = uint(unsafe.Sizeof(rectStruct{})) - 16
	_ uint = 80 - uint(unsafe.Sizeof(wndClassExW{}))
	_ uint = uint(unsafe.Sizeof(wndClassExW{})) - 80
	_ uint = 48 - uint(unsafe.Sizeof(msgStruct{}))
	_ uint = uint(unsafe.Sizeof(msgStruct{})) - 48
	_ uint = 40 - uint(unsafe.Sizeof(bitmapInfoHeader{}))
	_ uint = uint(unsafe.Sizeof(bitmapInfoHeader{})) - 40
	_ uint = 4 - uint(unsafe.Sizeof(blendFunction{}))
	_ uint = uint(unsafe.Sizeof(blendFunction{})) - 4
	_ uint = 32 - uint(unsafe.Sizeof(iconInfo{}))
	_ uint = uint(unsafe.Sizeof(iconInfo{})) - 32
	_ uint = 976 - uint(unsafe.Sizeof(notifyIconDataW{}))
	_ uint = uint(unsafe.Sizeof(notifyIconDataW{})) - 976
)

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func enableDPIAwareness() float64 {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorAwareV2)
	} else if procSetProcessDPIAware.Find() == nil {
		procSetProcessDPIAware.Call()
	}
	if procGetDpiForSystem.Find() == nil {
		dpi, _, _ := procGetDpiForSystem.Call()
		if dpi >= 72 && dpi <= 480 {
			return float64(dpi) / 96.0
		}
	}
	return 1.0
}

func getWorkAndScreenRects() (workArea, screen image.Rectangle) {
	var wr rectStruct
	procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wr)), 0)
	sw, _, _ := procGetSystemMetrics.Call(smCXScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCYScreen)
	screen = image.Rect(0, 0, int(sw), int(sh))
	workArea = image.Rect(int(wr.Left), int(wr.Top), int(wr.Right), int(wr.Bottom))
	if workArea.Empty() {
		workArea = image.Rect(0, 0, screen.Dx(), screen.Dy()-48)
	}
	return workArea, screen
}

func getCursorPos() (int32, int32) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt.X, pt.Y
}

// dibSurface holds a reusable top-down 32-bit premultiplied BGRA DIBSection for UpdateLayeredWindow.
type dibSurface struct {
	memDC  uintptr
	hBmp   uintptr
	oldBmp uintptr
	bits   unsafe.Pointer
	w, h   int
	buf    []byte
}

func newDIBSurface() *dibSurface {
	dc, _, _ := procCreateCompatibleDC.Call(0)
	return &dibSurface{memDC: dc}
}

func (s *dibSurface) ensureSize(needW, needH int) bool {
	if needW <= 0 || needH <= 0 {
		return false
	}
	if s.hBmp != 0 && s.w >= needW && s.h >= needH {
		return true
	}
	// Grow in 64px steps to minimize GDI reallocations
	newW := ((needW + 63) / 64) * 64
	newH := ((needH + 63) / 64) * 64
	if newW < s.w {
		newW = s.w
	}
	if newH < s.h {
		newH = s.h
	}

	var bi bitmapInfo
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = int32(newW)
	bi.BmiHeader.BiHeight = -int32(newH) // negative = top-down DIB
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = biRGB

	var bits unsafe.Pointer
	hBmp, _, _ := procCreateDIBSection.Call(
		s.memDC,
		uintptr(unsafe.Pointer(&bi)),
		dibRGBColor,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBmp == 0 || bits == nil {
		return false
	}

	if s.hBmp != 0 {
		procSelectObject.Call(s.memDC, s.oldBmp)
		procDeleteObject.Call(s.hBmp)
	}
	s.oldBmp, _, _ = procSelectObject.Call(s.memDC, hBmp)
	s.hBmp = hBmp
	s.bits = bits
	s.w = newW
	s.h = newH
	s.buf = unsafe.Slice((*byte)(bits), newW*newH*4)
	return true
}

func (s *dibSurface) close() {
	if s.hBmp != 0 {
		procSelectObject.Call(s.memDC, s.oldBmp)
		procDeleteObject.Call(s.hBmp)
		s.hBmp = 0
	}
	if s.memDC != 0 {
		procDeleteDC.Call(s.memDC)
		s.memDC = 0
	}
	s.buf = nil
}

// createIconFromSprite converts an NRGBA sprite into a crisp 32x32 Win32 HICON for the tray & window.
func createIconFromSprite(src *image.NRGBA) uintptr {
	const iconSz = 32
	var bi bitmapInfo
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = iconSz
	bi.BmiHeader.BiHeight = -iconSz
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = biRGB

	var bits unsafe.Pointer
	hColor, _, _ := procCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&bi)),
		dibRGBColor,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hColor == 0 || bits == nil {
		return 0
	}
	defer procDeleteObject.Call(hColor)

	dst := unsafe.Slice((*byte)(bits), iconSz*iconSz*4)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	scaleX := float64(sw) / float64(iconSz)
	scaleY := float64(sh) / float64(iconSz)

	for y := 0; y < iconSz; y++ {
		sy0 := int(math.Floor(float64(y) * scaleY))
		sy1 := int(math.Ceil(float64(y+1) * scaleY))
		if sy1 > sh {
			sy1 = sh
		}
		for x := 0; x < iconSz; x++ {
			sx0 := int(math.Floor(float64(x) * scaleX))
			sx1 := int(math.Ceil(float64(x+1) * scaleX))
			if sx1 > sw {
				sx1 = sw
			}
			var rS, gS, bS, aS, n float64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					p := src.NRGBAAt(sx, sy)
					af := float64(p.A) / 255.0
					rS += float64(p.R) * af
					gS += float64(p.G) * af
					bS += float64(p.B) * af
					aS += float64(p.A)
					n++
				}
			}
			if n > 0 && aS > 0 {
				a := aS / n
				af := a / 255.0
				idx := (y*iconSz + x) * 4
				// Non-premultiplied BGRA for 32-bit HICON color bitmap
				dst[idx+0] = clampU8(bS / n / af)
				dst[idx+1] = clampU8(gS / n / af)
				dst[idx+2] = clampU8(rS / n / af)
				dst[idx+3] = clampU8(a)
			}
		}
	}

	hMask, _, _ := procCreateBitmap.Call(iconSz, iconSz, 1, 1, 0)
	if hMask == 0 {
		return 0
	}
	defer procDeleteObject.Call(hMask)

	ii := iconInfo{
		FIcon:    1,
		HbmMask:  hMask,
		HbmColor: hColor,
	}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return hIcon
}

func clampU8(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(math.Round(v))
}

var (
	enumWinCb     = syscall.NewCallback(enumWindowsProc)
	enumSelfHwnd  uintptr
	enumWorkArea  image.Rectangle
	enumCollected []pet.WindowRect
)

func enumVisibleWindows(selfHwnd uintptr, workArea image.Rectangle) []pet.WindowRect {
	enumSelfHwnd = selfHwnd
	enumWorkArea = workArea
	enumCollected = make([]pet.WindowRect, 0, 12)
	procEnumWindows.Call(enumWinCb, 0)
	return enumCollected
}

func enumWindowsProc(hwnd, _ uintptr) uintptr {
	if len(enumCollected) >= 12 {
		return 0 // stop enumeration
	}
	if hwnd == 0 || hwnd == enumSelfHwnd {
		return 1
	}
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	if vis == 0 {
		return 1
	}
	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		return 1
	}
	exStyle, _, _ := procGetWindowLongW.Call(hwnd, gwlExStyle)
	if exStyle&(wsExToolWindow|wsExTransparent) != 0 {
		return 1
	}
	// Skip invisible/background helper windows with no window title
	titleLen, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if titleLen == 0 {
		return 1
	}

	// Skip cloaked UWP/background windows on Windows 10/11
	if procDwmGetWindowAttribute.Find() == nil {
		var cloaked uint32
		hr, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), 4)
		if hr == 0 && cloaked != 0 {
			return 1
		}
	}

	// Skip desktop & taskbar shell windows
	var clsBuf [64]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&clsBuf[0])), uintptr(len(clsBuf)))
	if n > 0 {
		cls := syscall.UTF16ToString(clsBuf[:n])
		switch cls {
		case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd",
			"Windows.UI.Core.CoreWindow", "ApplicationFrameInputSinkWindow", "ForegroundStaging":
			return 1
		}
	}

	// Get exact DWM visible border (excluding invisible Win10/11 7px drop-shadow margins!)
	var r rectStruct
	gotRect := false
	if procDwmGetWindowAttribute.Find() == nil {
		hr, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&r)), 16)
		if hr == 0 {
			gotRect = true
		}
	}
	if !gotRect {
		ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if ok == 0 {
			return 1
		}
	}

	rect := image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
	if rect.Dx() < 180 || rect.Dy() < 120 {
		return 1
	}
	if !rect.Overlaps(enumWorkArea) {
		return 1
	}
	// Keep maximized windows in Z-order as Occluding=true so hidden windows behind them are never climbed!
	isMaximized := rect.Min.X <= enumWorkArea.Min.X+4 && rect.Max.X >= enumWorkArea.Max.X-4 &&
		rect.Min.Y <= enumWorkArea.Min.Y+6 && rect.Max.Y >= enumWorkArea.Max.Y-6

	enumCollected = append(enumCollected, pet.WindowRect{
		ID:        hwnd,
		Rect:      rect,
		Occluding: isMaximized,
	})
	if isMaximized {
		// Nothing behind a fully maximized window is visible on this monitor; stop enumerating!
		return 0
	}
	return 1
}
