//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"chibipet/internal/pet"
)

const (
	timerIDTick = 1
	tickMs      = 25 // 40 FPS: smooth animations with ultra-low CPU (<1%)
	trayIconID  = 1001

	// Menu command IDs
	cmdSpawnBase   = 2000 // + index in pet.Catalog
	cmdSpawnRandom = 2100
	cmdSpawnAll    = 2101
	cmdPetReact    = 2200
	cmdPetDismiss  = 2201
	cmdScale85     = 2300
	cmdScale100    = 2301
	cmdScale125    = 2302
	cmdScale150    = 2303
	cmdToggleRoam  = 2400
	cmdToggleClimb = 2401
	cmdToggleTop   = 2402
	cmdUnstackAll  = 2500
	cmdReplayIntro = 2501
	cmdClearAll    = 2502
	cmdExit        = 2999
)

type desktopApp struct {
	hwnd             uintptr
	hInstance        uintptr
	hIcon            uintptr
	hCursorHand      uintptr
	hCursorGrab      uintptr
	hCursorArrow     uintptr
	classNamePtr     *uint16
	msgTaskbarCreate uint32

	world          *pet.World
	surface        *dibSurface
	lastTick       time.Time
	topMostCounter int
	winPollCounter int
	mouseCaptured  bool
	visible        bool
	inMenu         bool
}

var app *desktopApp

func main() {
	runtime.LockOSThread()

	// Single-instance check
	mutexName := utf16Ptr("Local\\ChibiDesktopPet_SingleInstance_v1")
	hMutex, _, lastErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if hMutex != 0 && lastErr == syscall.Errno(errorAlreadyExists) {
		procMessageBoxW.Call(
			0,
			uintptr(unsafe.Pointer(utf16Ptr("이미 치비 데스크톱 펫이 실행 중입니다!\n작업표시줄 오른쪽 트레이 아이콘을 우클릭해 메뉴를 열어주세요."))),
			uintptr(unsafe.Pointer(utf16Ptr("치비 데스크톱 펫"))),
			mbOK|mbIconInformation,
		)
		return
	}

	dpiScale := enableDPIAwareness()
	bank, err := pet.LoadSpriteBank()
	if err != nil {
		procMessageBoxW.Call(
			0,
			uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("캐릭터 이미지를 불러오지 못했습니다:\n%v", err)))),
			uintptr(unsafe.Pointer(utf16Ptr("치비 데스크톱 펫 오류"))),
			mbOK,
		)
		return
	}

	workArea, screen := getWorkAndScreenRects()
	hInst, _, _ := procGetModuleHandleW.Call(0)

	hHand, _, _ := procLoadCursorW.Call(0, idcHand)
	hGrab, _, _ := procLoadCursorW.Call(0, idcSizeAll)
	hArrow, _, _ := procLoadCursorW.Call(0, idcArrow)

	// Load embedded app icon resource (ID 1) for tray & window, falling back to runtime sprite icon
	hIcon, _, _ := procLoadImageW.Call(hInst, 1, 1, 32, 32, 0) // IMAGE_ICON=1
	if hIcon == 0 {
		if ss := bank.ByID["manggom"]; ss != nil {
			hIcon = createIconFromSprite(ss.Poses[pet.PoseIdle])
		}
	}

	className := utf16Ptr("ChibiDesktopPetOverlayClass")
	tbMsg, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))

	app = &desktopApp{
		hInstance:        hInst,
		hIcon:            hIcon,
		hCursorHand:      hHand,
		hCursorGrab:      hGrab,
		hCursorArrow:     hArrow,
		classNamePtr:     className,
		msgTaskbarCreate: uint32(tbMsg),
		world:            pet.NewWorld(bank, workArea, screen, dpiScale),
		surface:          newDIBSurface(),
		lastTick:         time.Now(),
		visible:          true,
	}

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		HIcon:         hIcon,
		HIconSm:       hIcon,
		HCursor:       hHand,
		LpszClassName: className,
	}
	if atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return
	}

	title := utf16Ptr("치비 데스크톱 펫")
	exStyle := uintptr(wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate)
	style := uintptr(wsPopup | wsVisible)
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		style,
		0, 0, 1, 1,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return
	}
	app.hwnd = hwnd

	app.addTrayIcon()
	app.renderFrame()
	procSetTimer.Call(hwnd, timerIDTick, tickMs, 0)

	var msg msgStruct
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (a *desktopApp) addTrayIcon() {
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = a.hwnd
	nid.UID = trayIconID
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmTrayIcon
	nid.HIcon = a.hIcon
	tip, _ := syscall.UTF16FromString("치비 데스크톱 펫 (클릭: 메뉴)")
	copy(nid.SzTip[:], tip)
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func (a *desktopApp) removeTrayIcon() {
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = a.hwnd
	nid.UID = trayIconID
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func (a *desktopApp) renderFrame() {
	bounds := a.world.RenderBounds()
	if bounds.Empty() {
		// Render a 1x1 fully transparent pixel instead of hiding HWND so tray TrackPopupMenu
		// SetForegroundWindow always works reliably even when all pets are cleared.
		if a.visible && a.surface.ensureSize(1, 1) {
			clear(a.surface.buf[:4])
			ptDst := point{X: 0, Y: 0}
			sz := sizeStruct{CX: 1, CY: 1}
			ptSrc := point{X: 0, Y: 0}
			blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
			procUpdateLayeredWindow.Call(
				a.hwnd, 0,
				uintptr(unsafe.Pointer(&ptDst)),
				uintptr(unsafe.Pointer(&sz)),
				a.surface.memDC,
				uintptr(unsafe.Pointer(&ptSrc)),
				0,
				uintptr(unsafe.Pointer(&blend)),
				ulwAlpha,
			)
			a.visible = false
		}
		return
	}

	bw, bh := bounds.Dx(), bounds.Dy()
	if !a.surface.ensureSize(bw, bh) {
		return
	}

	strideBytes := a.surface.w * 4
	a.world.RenderBGRAWithStride(a.surface.buf, strideBytes, bw, bh, bounds.Min.X, bounds.Min.Y)

	ptDst := point{X: int32(bounds.Min.X), Y: int32(bounds.Min.Y)}
	sz := sizeStruct{CX: int32(bw), CY: int32(bh)}
	ptSrc := point{X: 0, Y: 0}
	blend := blendFunction{
		BlendOp:             acSrcOver,
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         acSrcAlpha,
	}

	procUpdateLayeredWindow.Call(
		a.hwnd,
		0,
		uintptr(unsafe.Pointer(&ptDst)),
		uintptr(unsafe.Pointer(&sz)),
		a.surface.memDC,
		uintptr(unsafe.Pointer(&ptSrc)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
	a.visible = true
}

func (a *desktopApp) applyTopMost() {
	z := hwndNoTopmost
	if a.world.TopMost {
		z = hwndTopmost
	}
	procSetWindowPos.Call(a.hwnd, z, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

func (a *desktopApp) showContextMenu(screenX, screenY int32, targetPetID int) {
	if a.inMenu {
		return
	}
	a.inMenu = true
	defer func() { a.inMenu = false }()

	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	// Submenu: Summon characters with propeller claw (prevent duplicates!)
	hSpawn, _, _ := procCreatePopupMenu.Call()
	prevGroup := ""
	remaining := 0
	for i, def := range pet.Catalog {
		if prevGroup != "" && def.Group != prevGroup {
			procAppendMenuW.Call(hSpawn, mfSeparator, 0, 0)
		}
		prevGroup = def.Group
		flags := uintptr(mfString)
		label := fmt.Sprintf("[%s] %s", def.Group, def.Name)
		if a.world.HasCharacter(def.ID) {
			flags |= mfGrayed | mfChecked
			label += " (소환됨)"
		} else {
			remaining++
		}
		procAppendMenuW.Call(hSpawn, flags, uintptr(cmdSpawnBase+i), uintptr(unsafe.Pointer(utf16Ptr(label))))
	}
	procAppendMenuW.Call(hSpawn, mfSeparator, 0, 0)
	bulkFlags := uintptr(mfString)
	if remaining == 0 {
		bulkFlags |= mfGrayed
	}
	procAppendMenuW.Call(hSpawn, bulkFlags, cmdSpawnRandom, uintptr(unsafe.Pointer(utf16Ptr("랜덤 캐릭터 소환"))))
	procAppendMenuW.Call(hSpawn, bulkFlags, cmdSpawnAll, uintptr(unsafe.Pointer(utf16Ptr("남은 캐릭터 전원 소환"))))

	procAppendMenuW.Call(hMenu, mfPopup, hSpawn, uintptr(unsafe.Pointer(utf16Ptr("캐릭터 소환 (프로펠러 집게)"))))

	if targetPetID != 0 {
		if p := a.world.FindPet(targetPetID); p != nil {
			charName := p.CharID
			if ss := a.world.Bank.ByID[p.CharID]; ss != nil {
				charName = ss.Def.Name
			}
			procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
			procAppendMenuW.Call(hMenu, mfString, cmdPetReact, uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("%s 특별 리액션 보기", charName)))))
			procAppendMenuW.Call(hMenu, mfString, cmdPetDismiss, uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("%s 집게로 보내기", charName)))))
		}
	}

	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)

	// Submenu: Scale
	hScale, _, _ := procCreatePopupMenu.Call()
	scaleFlags := func(target float64) uintptr {
		if a.world.Scale > target-0.05 && a.world.Scale < target+0.05 {
			return mfString | mfChecked
		}
		return mfString
	}
	procAppendMenuW.Call(hScale, scaleFlags(0.85), cmdScale85, uintptr(unsafe.Pointer(utf16Ptr("작게 (85%)"))))
	procAppendMenuW.Call(hScale, scaleFlags(1.00), cmdScale100, uintptr(unsafe.Pointer(utf16Ptr("보통 (100%)"))))
	procAppendMenuW.Call(hScale, scaleFlags(1.25), cmdScale125, uintptr(unsafe.Pointer(utf16Ptr("크게 (125%)"))))
	procAppendMenuW.Call(hScale, scaleFlags(1.50), cmdScale150, uintptr(unsafe.Pointer(utf16Ptr("아주 크게 (150%)"))))
	procAppendMenuW.Call(hMenu, mfPopup, hScale, uintptr(unsafe.Pointer(utf16Ptr("캐릭터 크기"))))

	roamFlag := uintptr(mfString)
	if a.world.FreeRoam {
		roamFlag |= mfChecked
	}
	procAppendMenuW.Call(hMenu, roamFlag, cmdToggleRoam, uintptr(unsafe.Pointer(utf16Ptr("자유롭게 돌아다니기"))))

	climbFlag := uintptr(mfString)
	if a.world.WindowClimb {
		climbFlag |= mfChecked
	}
	procAppendMenuW.Call(hMenu, climbFlag, cmdToggleClimb, uintptr(unsafe.Pointer(utf16Ptr("창 타고 오르기 / 뛰어내리기"))))

	topFlag := uintptr(mfString)
	if a.world.TopMost {
		topFlag |= mfChecked
	}
	procAppendMenuW.Call(hMenu, topFlag, cmdToggleTop, uintptr(unsafe.Pointer(utf16Ptr("항상 위(작업표시줄 위) 유지"))))

	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, cmdUnstackAll, uintptr(unsafe.Pointer(utf16Ptr("쌓인 캐릭터 모두 내리기"))))
	procAppendMenuW.Call(hMenu, mfString, cmdReplayIntro, uintptr(unsafe.Pointer(utf16Ptr("오프닝 다시 보기 (망곰 + 탄지로)"))))
	procAppendMenuW.Call(hMenu, mfString, cmdClearAll, uintptr(unsafe.Pointer(utf16Ptr("모두 정리하기"))))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, cmdExit, uintptr(unsafe.Pointer(utf16Ptr("종료"))))

	// Official Win32 pattern (KB Q135788) so clicking outside dismisses the menu reliably
	procSetForegroundWindow.Call(a.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		tpmRightButton|tpmReturnCmd|tpmNonotify|tpmBottomAlign,
		uintptr(screenX),
		uintptr(screenY),
		0,
		a.hwnd,
		0,
	)
	procPostMessageW.Call(a.hwnd, wmNull, 0, 0)

	a.handleCommand(int(cmd), targetPetID)
}

func (a *desktopApp) handleCommand(cmd int, targetPetID int) {
	if cmd == 0 {
		return
	}
	if cmd >= cmdSpawnBase && cmd < cmdSpawnBase+len(pet.Catalog) {
		def := pet.Catalog[cmd-cmdSpawnBase]
		// If summoned via right-clicking a pet, stack onto that pet or drop nearby!
		a.world.SpawnWithClaw(def.ID, 0, 0)
		return
	}
	switch cmd {
	case cmdSpawnRandom:
		a.world.SpawnRandomWithClaw()
	case cmdSpawnAll:
		a.world.SpawnAllEight()
	case cmdPetReact:
		if targetPetID != 0 {
			a.world.TriggerReact(targetPetID)
		}
	case cmdPetDismiss:
		if targetPetID != 0 {
			a.world.DismissPet(targetPetID)
		}
	case cmdScale85:
		a.world.Scale = 0.85
	case cmdScale100:
		a.world.Scale = 1.00
	case cmdScale125:
		a.world.Scale = 1.25
	case cmdScale150:
		a.world.Scale = 1.50
	case cmdToggleRoam:
		a.world.FreeRoam = !a.world.FreeRoam
	case cmdToggleClimb:
		a.world.WindowClimb = !a.world.WindowClimb
	case cmdToggleTop:
		a.world.TopMost = !a.world.TopMost
		a.applyTopMost()
	case cmdUnstackAll:
		a.world.UnstackAll()
	case cmdReplayIntro:
		a.world.StartClipIntro()
	case cmdClearAll:
		a.world.ClearAll()
	case cmdExit:
		procDestroyWindow.Call(a.hwnd)
	}
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if app != nil && app.msgTaskbarCreate != 0 && msg == app.msgTaskbarCreate {
		app.addTrayIcon()
		return 0
	}

	switch msg {
	case wmTimer:
		if wParam == timerIDTick && app != nil {
			app.winPollCounter++
			if app.winPollCounter >= 8 { // refresh open Windows app rects every ~200ms
				app.winPollCounter = 0
				if app.world.WindowClimb {
					wins := enumVisibleWindows(app.hwnd, app.world.WorkArea)
					app.world.SetWindows(wins)
				} else if len(app.world.Windows) > 0 {
					app.world.SetWindows(nil)
				}
			}

			now := time.Now()
			dt := now.Sub(app.lastTick).Seconds()
			app.lastTick = now
			app.world.Step(dt)
			app.renderFrame()

			app.topMostCounter++
			if app.world.TopMost && app.visible && !app.inMenu && app.topMostCounter >= 80 {
				app.topMostCounter = 0
				app.applyTopMost()
			}
		}
		return 0

	case wmLButtonDown:
		if app != nil {
			cx, cy := getCursorPos()
			if hit := app.world.MouseDown(float64(cx), float64(cy)); hit != 0 {
				procSetCapture.Call(hwnd)
				app.mouseCaptured = true
			}
		}
		return 0

	case wmMouseMove:
		if app != nil {
			cx, cy := getCursorPos()
			if app.mouseCaptured {
				app.world.MouseMove(float64(cx), float64(cy))
			} else {
				app.world.MouseHover(float64(cx), float64(cy))
			}
		}
		return 0

	case wmLButtonUp:
		if app != nil && app.mouseCaptured {
			app.mouseCaptured = false
			procReleaseCapture.Call()
			cx, cy := getCursorPos()
			app.world.MouseUp(float64(cx), float64(cy))
		}
		return 0

	case wmCaptureChanged:
		if app != nil && app.mouseCaptured {
			app.mouseCaptured = false
			cx, cy := getCursorPos()
			app.world.MouseUp(float64(cx), float64(cy))
		}
		return 0

	case wmRButtonUp:
		if app != nil {
			cx, cy := getCursorPos()
			hit := app.world.HitTest(float64(cx), float64(cy))
			app.showContextMenu(cx, cy, hit)
		}
		return 0

	case wmSetCursor:
		if app != nil {
			if app.world.IsDragging() {
				procSetCursor.Call(app.hCursorGrab)
			} else {
				procSetCursor.Call(app.hCursorHand)
			}
			return 1
		}

	case wmTrayIcon:
		if app != nil {
			ev := uint32(lParam & 0xFFFF)
			if ev == wmRButtonUp || ev == wmLButtonUp {
				cx, cy := getCursorPos()
				app.showContextMenu(cx, cy, 0)
			}
		}
		return 0

	case wmDisplayChange, wmSettingChange:
		if app != nil {
			workArea, screen := getWorkAndScreenRects()
			app.world.SetWorkArea(workArea, screen)
		}
		return 0

	case wmDestroy:
		if app != nil {
			procKillTimer.Call(hwnd, timerIDTick)
			app.removeTrayIcon()
			if app.surface != nil {
				app.surface.close()
			}
			if app.hIcon != 0 {
				procDestroyIcon.Call(app.hIcon)
				app.hIcon = 0
			}
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}
