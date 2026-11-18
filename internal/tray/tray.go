package tray

import (
	"bytes"
	"errors"
	"image/png"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	className = "CodeBuddyUsageTrayWnd"
	trayUID   = 1001
	wmTrayMsg = 0x0400 + 1
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	wmRButtonUp = 0x0205
	wmLButtonUp = 0x0202
	wmLButtonDbl = 0x0203
	wmMouseMove = 0x0200
	wmDestroy   = 0x0002
	wmCommand   = 0x0111

	wsOverlappedWindow = 0x10000000
	swHide             = 0
	idcArrow           = 32512
	mfString           = 0x00000000
	mfSeparator        = 0x00000000
	tpmRightAlign      = 0x0008

	nifInfoNoSound = 0x00000001
	lrDefaultColor = 0x00000004
)

// Callbacks 是托盘事件回调。
type Callbacks struct {
	OnClick    func()
	OnRight    func(x, y int32)
	OnHover    func()
	OnRefresh  func()
	OnClaim    func()
	OnDepart   func()
	OnSettings func()
	OnFix      func()
	OnOpen     func()
	OnQuit     func()
}

// Tray 封装 Shell_NotifyIcon 托盘图标与消息窗口。
type Tray struct {
	cb       *Callbacks
	hwnd     uintptr
	mu       sync.Mutex
	stopped  bool
	lastIcon []byte
	added    bool
	menuIDs  map[uint16]func()
}

var (
	user32    = windows.NewLazySystemDLL("user32.dll")
	shell32   = windows.NewLazySystemDLL("shell32.dll")
	gdi32     = windows.NewLazySystemDLL("gdi32.dll")
	procRegisterClass    = user32.NewProc("RegisterClassW")
	procCreateWindow     = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procShowWindow       = user32.NewProc("ShowWindow")
	procCreateIconFromRes = user32.NewProc("CreateIconFromResourceEx")
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procShellGetRect     = shell32.NewProc("Shell_NotifyIconGetRect")
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type notifyIconData struct {
	Size            uint32
	HWnd            uintptr
	UID             uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonHWND     uintptr
}

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

// New 创建托盘并注册消息窗口。
func New(cb *Callbacks) (*Tray, error) {
	if cb == nil {
		return nil, errors.New("nil callbacks")
	}
	t := &Tray{cb: cb, menuIDs: map[uint16]func(){}}

	classNamePtr, _ := windows.UTF16PtrFromString(className)
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	instance, _ := windows.GetCurrentProcess()
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   windows.NewCallback(t.wndProc),
		Instance:  uintptr(instance),
		Cursor:    cursor,
		ClassName: classNamePtr,
	}
	procRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := windows.UTF16PtrFromString("CodeBuddyUsageTray")
	hwnd, _, callErr := procCreateWindow.Call(
		0,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		0, 0, 0, 0,
		0, 0, uintptr(instance), 0,
	)
	if hwnd == 0 {
		return nil, callErr
	}
	t.hwnd = hwnd
	procShowWindow.Call(hwnd, swHide)
	return t, nil
}

func (t *Tray) wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmTrayMsg:
		switch uint32(lParam) {
		case wmLButtonUp, wmLButtonDbl:
			if t.cb.OnClick != nil {
				t.cb.OnClick()
			}
		case wmRButtonUp:
			var p point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			if t.cb.OnRight != nil {
				t.cb.OnRight(p.X, p.Y)
			}
		case wmMouseMove:
			if t.cb.OnHover != nil {
				t.cb.OnHover()
			}
		}
	case wmCommand:
		id := uint16(wParam)
		if fn := t.menuIDs[id]; fn != nil {
			fn()
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func utf16Copy(dst *[128]uint16, s string) {
	copy(dst[:], windows.StringToUTF16(s))
}
func utf16Copy256(dst *[256]uint16, s string) {
	copy(dst[:], windows.StringToUTF16(s))
}
func utf16Copy64(dst *[64]uint16, s string) {
	copy(dst[:], windows.StringToUTF16(s))
}

// SetIcon 更新托盘图标（仅数值变化时才实际更新）。
func (t *Tray) SetIcon(label string, state IconState) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return nil
	}
	pngBytes := RenderIcon(label, state)
	if bytes.Equal(pngBytes, t.lastIcon) {
		return nil
	}
	t.lastIcon = pngBytes
	hIcon, err := createIconFromPNG(pngBytes)
	if err != nil {
		return err
	}
	nid := notifyIconData{
		Size:            uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:            t.hwnd,
		UID:             trayUID,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: wmTrayMsg,
		Icon:            hIcon,
	}
	utf16Copy(&nid.Tip, "CodeBuddy Usage: "+label)
	action := uintptr(nimAdd)
	if t.added {
		action = nimModify
	}
	ret, _, callErr := procShellNotifyIcon.Call(action, uintptr(unsafe.Pointer(&nid)))
	procDestroyIcon.Call(hIcon)
	if ret == 0 {
		if action == nimAdd {
			ret2, _, _ := procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
			if ret2 != 0 {
				t.added = true
				return nil
			}
		}
		return callErr
	}
	t.added = true
	return nil
}

var procDestroyIcon = user32.NewProc("DestroyIcon")

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// createIconFromPNG 把 PNG 解码后构造成 32bpp DIB（BITMAPINFOHEADER + BGRA 像素 + AND 掩码），
// 再用 CreateIconFromResourceEx 创建 HICON。CreateIconFromResourceEx 不接受 PNG 原始字节。
func createIconFromPNG(pngBytes []byte) (uintptr, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return 0, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	headerSize := int(unsafe.Sizeof(bitmapInfoHeader{}))
	xorSize := w * h * 4
	andStride := ((w + 31) / 32) * 4
	andSize := andStride * h
	buf := make([]byte, headerSize+xorSize+andSize)
	bh := (*bitmapInfoHeader)(unsafe.Pointer(&buf[0]))
	bh.Size = uint32(headerSize)
	bh.Width = int32(w)
	bh.Height = int32(h * 2)
	bh.Planes = 1
	bh.BitCount = 32
	bh.Compression = 0
	bh.SizeImage = uint32(xorSize + andSize)

	pixels := buf[headerSize : headerSize+xorSize]
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			idx := ((h - 1 - y) * w + x) * 4
			pixels[idx] = byte(b >> 8)
			pixels[idx+1] = byte(g >> 8)
			pixels[idx+2] = byte(r >> 8)
			pixels[idx+3] = byte(a >> 8)
		}
	}
	ret, _, callErr := procCreateIconFromRes.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		1,
		0x00030000,
		uintptr(w), uintptr(h),
		lrDefaultColor,
	)
	if ret == 0 {
		return 0, callErr
	}
	return ret, nil
}

// SetTooltip 更新托盘提示文本。
func (t *Tray) SetTooltip(text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	nid := notifyIconData{
		Size:  uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:  t.hwnd,
		UID:   trayUID,
		Flags: nifTip,
	}
	utf16Copy(&nid.Tip, text)
	ret, _, err := procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return err
	}
	return nil
}

// ShowBalloon 显示托盘气泡通知。
func (t *Tray) ShowBalloon(title, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	nid := notifyIconData{
		Size:       uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:       t.hwnd,
		UID:        trayUID,
		Flags:      nifInfo,
		InfoFlags:  nifInfoNoSound,
	}
	utf16Copy64(&nid.InfoTitle, title)
	utf16Copy256(&nid.Info, message)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

// RemoveIcon 从系统托盘移除图标（NIM_DELETE）。退出前必须调用，否则残留僵尸图标。
func (t *Tray) RemoveIcon() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || !t.added {
		return
	}
	nid := notifyIconData{
		Size: uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd: t.hwnd,
		UID:  trayUID,
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	t.added = false
}

// GetIconRect 返回托盘图标屏幕矩形，用于定位悬浮卡。
func (t *Tray) GetIconRect() (int32, int32, int32, int32, bool) {
	var r rect
	id := struct {
		Size  uint32
		HWnd  uintptr
		UID   uint32
		Guid  windows.GUID
	}{Size: uint32(unsafe.Sizeof(struct {
		Size uint32
		HWnd uintptr
		UID  uint32
		Guid windows.GUID
	}{})), HWnd: t.hwnd, UID: trayUID}
	ret, _, _ := procShellGetRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&r)))
	if ret != 0 {
		return 0, 0, 0, 0, false
	}
	return r.Left, r.Top, r.Right, r.Bottom, true
}

// Run 运行托盘消息循环（阻塞）。
func (t *Tray) Run() error {
	var msg struct {
		HWnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      point
	}
	for {
		ret, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) {
			t.mu.Lock()
			t.stopped = true
			t.mu.Unlock()
			if ret == ^uintptr(0) {
				return callErr
			}
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Stop 结束消息循环。
func (t *Tray) Stop() {
	t.mu.Lock()
	if !t.stopped {
		procDestroyWindow.Call(t.hwnd)
	}
	t.mu.Unlock()
}

// PopupMenu 在指定屏幕坐标弹出右键菜单。
func (t *Tray) PopupMenu(x, y int32) {
	t.mu.Lock()
	menu, _, _ := procCreatePopupMenu.Call()
	ids := map[uint16]func(){}
	items := []struct {
		id   uint16
		text string
		fn   func()
	}{
		{1, "刷新", t.cb.OnRefresh},
		{2, "领积分", t.cb.OnClaim},
		{3, "去旅行", t.cb.OnDepart},
		{0, "-", nil},
		{4, "设置", t.cb.OnSettings},
		{5, "修复凭据", t.cb.OnFix},
		{6, "打开用量页", t.cb.OnOpen},
		{0, "-", nil},
		{7, "退出", t.cb.OnQuit},
	}
	for _, item := range items {
		if item.text == "-" {
			procAppendMenu.Call(menu, mfSeparator, 0, 0)
			continue
		}
		text, _ := windows.UTF16PtrFromString(item.text)
		procAppendMenu.Call(menu, mfString, uintptr(item.id), uintptr(unsafe.Pointer(text)))
		ids[item.id] = item.fn
	}
	t.menuIDs = ids
	t.mu.Unlock()
	procSetForeground.Call(t.hwnd)
	procTrackPopupMenu.Call(menu, tpmRightAlign, uintptr(x), uintptr(y), 0, t.hwnd, 0)
	procDestroyMenu.Call(menu)
}
