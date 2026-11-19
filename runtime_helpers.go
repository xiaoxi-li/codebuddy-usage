package main

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const mainWindowClass = "CodeBuddyUsageMainWnd"

var (
	user32Dll          = windows.NewLazySystemDLL("user32.dll")
	gdi32Dll           = windows.NewLazySystemDLL("gdi32.dll")
	procFindWindowW    = user32Dll.NewProc("FindWindowW")
	procGetWindowRect  = user32Dll.NewProc("GetWindowRect")
	procCreateRoundRgn = gdi32Dll.NewProc("CreateRoundRectRgn")
	procSetWindowRgn   = user32Dll.NewProc("SetWindowRgn")
	procDeleteObject   = gdi32Dll.NewProc("DeleteObject")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

// applyRoundCorners 用 Win32 区域把主窗口裁成圆角，
// 彻底消除 WebView2 透明窗口在四角露出的白色矩形边角。
func applyRoundCorners() error {
	className, _ := windows.UTF16PtrFromString(mainWindowClass)
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return errors.New("main window not found")
	}
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return errors.New("invalid window size")
	}
	// 圆角半径：CSS 14px，按 DPI 缩放比例取 14/422 的宽度占比，最小 8
	radius := w * 14 / 422
	if radius < 8 {
		radius = 8
	}
	hrgn, _, _ := procCreateRoundRgn.Call(0, 0, uintptr(w), uintptr(h), uintptr(radius), uintptr(radius))
	if hrgn == 0 {
		return errors.New("CreateRoundRectRgn failed")
	}
	// SetWindowRgn 成功后系统会接管 hrgn，不应再 DeleteObject
	ret, _, callErr := procSetWindowRgn.Call(hwnd, hrgn, 1)
	if ret == 0 {
		procDeleteObject.Call(hrgn)
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return callErr
		}
		return errors.New("SetWindowRgn failed")
	}
	return nil
}

func wailsEventsEmit(ctx context.Context, name string, data ...interface{}) {
	runtime.EventsEmit(ctx, name, data...)
}

func windowShow(ctx context.Context) {
	runtime.WindowShow(ctx)
}

func windowHide(ctx context.Context) {
	runtime.WindowHide(ctx)
}

func windowSetPosition(ctx context.Context, x, y int) {
	runtime.WindowSetPosition(ctx, x, y)
}

func windowMinimize(ctx context.Context) {
	runtime.WindowMinimise(ctx)
}

func appQuit(ctx context.Context) {
	runtime.Quit(ctx)
}

func openBrowser(url string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
