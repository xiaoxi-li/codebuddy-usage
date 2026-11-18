package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"

	"codebuddy-usage-desktop/internal/api"
	"codebuddy-usage-desktop/internal/auth"
	"codebuddy-usage-desktop/internal/core"
	"codebuddy-usage-desktop/internal/tray"
)

//go:embed all:frontend
var frontendAssets embed.FS

func main() {
	initLogger()

	appData := core.AppDataDir()
	settings := core.NewSettingsStore(appData)
	tokenStore := &auth.FileTokenStore{Path: filepath.Join(appData, "token.bin")}
	manager := auth.NewManager(tokenStore)
	client := api.NewClient(manager,
		func() string { return settings.Get().AccessToken },
		func() string { return settings.Get().ApiBase },
	)

	app := NewApp(settings, manager, client)

	err := wails.Run(&options.App{
		Title:             "CodeBuddy Usage",
		Width:             422,
		Height:            470,
		Frameless:         true,
		AlwaysOnTop:       true,
		HideWindowOnClose: true,
		Assets:            frontendAssets,
		BackgroundColour:  &options.RGBA{R: 16, G: 21, B: 31, A: 0},
		Windows: &wailswindows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         wailswindows.None,
			WindowClassName:      "CodeBuddyUsageMainWnd",
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
	})
	if err != nil {
		slog.Error("wails run failed", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initLogger() {
	dir := core.AppDataDir()
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		return
	}
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
}

// App 是 Wails 绑定对象。
type App struct {
	ctx        context.Context
	settings   *core.SettingsStore
	manager    *auth.Manager
	client     *api.Client
	orch       *core.Orchestrator
	tray       *tray.Tray
	runtimeAPI interface{}

	mu             sync.Mutex
	hoverAt        time.Time
	shownAt        time.Time
	visible        atomic.Bool
	pointerInside  atomic.Bool
	userPositioned atomic.Bool // 用户拖动过窗口后，不再自动定位到托盘上方
}

func NewApp(settings *core.SettingsStore, manager *auth.Manager, client *api.Client) *App {
	return &App{settings: settings, manager: manager, client: client}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	slog.Info("app starting")
	windowHide(ctx)
	// WebView2 透明窗口的 CSS 圆角外仍会露出矩形白色边角，
	// 需在 Win32 层用 SetWindowRgn 把窗口本身裁成圆角。
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := applyRoundCorners(); err != nil {
			slog.Warn("apply round corners failed", "error", err)
		} else {
			slog.Info("window rounded corners applied")
		}
	}()

	trayCallbacks := &tray.Callbacks{
		OnClick:    func() { a.showCardNearTray() },
		OnRight:    func(x, y int32) { a.showMenuAt(x, y) },
		// 不再响应托盘悬浮：托盘图标常驻显示余量数字，点击才弹明细卡。
		OnRefresh:  func() { go a.orch.Refresh() },
		OnClaim:    func() { go a.Claim() },
		OnDepart:   func() { go a.Depart() },
		OnSettings: func() { a.emitEvent("show-settings", "") },
		OnFix:      func() { a.FixAuth() },
		OnOpen:     func() { a.OpenUsagePage() },
		OnQuit:     func() { go a.Quit() },
	}
	t, err := tray.New(trayCallbacks)
	if err != nil {
		slog.Error("tray init failed", "error", err)
		return
	}
	a.tray = t

	a.orch = core.NewOrchestrator(a.settings, a.manager, a.client, t)
	a.orch.SetEmit(func(s core.UsageSnapshot) {
		a.emitEvent("usage-update", s.ToJSON())
	})
	a.orch.SetShowCard(a.showCardNearTray)

	go func() {
		if err := t.Run(); err != nil {
			slog.Error("tray loop ended", "error", err)
		}
	}()
	a.orch.Start()
}

func (a *App) shutdown(ctx context.Context) {
	if a.orch != nil {
		a.orch.Stop()
	}
	if a.tray != nil {
		a.tray.RemoveIcon()
		a.tray.Stop()
	}
	slog.Info("app stopped")
}

func (a *App) emitEvent(name, payload string) {
	if a.ctx == nil {
		return
	}
	// Wails runtime.EventsOn/emit 通过全局函数注入
	wailsEventsEmit(a.ctx, name, payload)
}

func (a *App) showCardNearTray() {
	if a.tray == nil || a.ctx == nil {
		return
	}
	// 用户手动拖动过窗口后，尊重其位置，不再弹回托盘上方。
	if !a.userPositioned.Load() {
		_, top, _, _, ok := a.tray.GetIconRect()
		if ok {
			// 卡片高 470，定位到托盘上方；居中于托盘横坐标
			x, y := a.trayCenter(), int(top)-478
			if y < 8 {
				y = 8
			}
			windowSetPosition(a.ctx, x, y)
		}
	}
	windowShow(a.ctx)
	a.visible.Store(true)
	a.mu.Lock()
	now := time.Now()
	a.hoverAt = now
	a.shownAt = now
	a.mu.Unlock()
	a.pointerInside.Store(false)
	// 明细卡改为点击显示后保持常驻，直到用户最小化/关闭/再次点击托盘。
}

func (a *App) trayCenter() int {
	l, _, r, _, ok := a.tray.GetIconRect()
	if !ok {
		return 0
	}
	return int((l + r) / 2) - 211
}

func (a *App) showMenuAt(x, y int32) {
	if a.tray != nil {
		a.tray.PopupMenu(x, y)
	}
}

// ===== Wails 绑定方法 =====

func (a *App) GetSnapshot() string {
	if a.orch == nil {
		return ""
	}
	return a.orch.Snapshot().ToJSON()
}

func (a *App) Refresh() {
	if a.orch != nil {
		a.orch.Refresh()
	}
}

func (a *App) Claim() map[string]interface{} {
	if a.orch == nil {
		return map[string]interface{}{"error": "应用尚未就绪"}
	}
	r, err := a.orch.Claim()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return r
}

func (a *App) Depart() map[string]interface{} {
	if a.orch == nil {
		return map[string]interface{}{"error": "应用尚未就绪"}
	}
	r, err := a.orch.Depart()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return r
}

func (a *App) GetSettings() string {
	b, _ := json.Marshal(a.settings.Get())
	return string(b)
}

func (a *App) SaveSettings(raw string) bool {
	var s core.Settings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		slog.Warn("invalid settings payload", "error", err)
		return false
	}
	if err := a.settings.Save(s); err != nil {
		slog.Warn("save settings failed", "error", err)
		return false
	}
	if err := core.SetAutostart(s.Autostart); err != nil {
		slog.Warn("set autostart failed", "error", err)
	}
	return true
}

func (a *App) FixAuth() {
	if a.orch != nil {
		a.orch.FixAuth()
	}
}

func (a *App) OpenUsagePage() {
	base := a.settings.Get().ApiBase
	if base == "" {
		base = "https://www.workbuddy.cn"
	}
	_ = openBrowser(base + "/profile/plans-usage")
}

func (a *App) KeepVisible() {
	a.mu.Lock()
	a.hoverAt = time.Now()
	a.mu.Unlock()
}

func (a *App) SetPointerInside(inside bool) {
	a.pointerInside.Store(inside)
	if inside {
		a.mu.Lock()
		a.hoverAt = time.Now()
		a.mu.Unlock()
	}
}

// MarkMoved 由前端在标题栏拖动时调用，用于记住用户手动定位的窗口位置。
func (a *App) MarkMoved() {
	a.userPositioned.Store(true)
}

// Minimize 最小化窗口到任务栏（程序继续常驻，可从任务栏恢复）。
func (a *App) Minimize() {
	if a.ctx == nil {
		return
	}
	windowMinimize(a.ctx)
}

// CloseWindow 关闭窗口：完全退出程序并清理托盘图标。
func (a *App) CloseWindow() {
	a.Quit()
}

// ResetPosition 清除手动定位，下次显示时重新贴近托盘。
func (a *App) ResetPosition() {
	a.userPositioned.Store(false)
}

// Quit 退出程序：先移除托盘图标，再结束 Wails 应用，避免残留僵尸图标。
func (a *App) Quit() {
	if a.tray != nil {
		a.tray.RemoveIcon()
	}
	if a.ctx != nil {
		appQuit(a.ctx)
	}
}
