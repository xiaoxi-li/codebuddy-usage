package core

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"codebuddy-usage-desktop/internal/api"
	"codebuddy-usage-desktop/internal/auth"
	"codebuddy-usage-desktop/internal/service"
	"codebuddy-usage-desktop/internal/tray"
)

// Orchestrator 负责定时刷新、并发锁、通知与快照分发。
type Orchestrator struct {
	settings  *SettingsStore
	manager   *auth.Manager
	client    *api.Client
	buddy     *service.BuddyService
	tray      *tray.Tray

	mu       sync.RWMutex
	snapshot UsageSnapshot
	updating atomic.Bool

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	// 由 Wails App 注入
	emit    func(UsageSnapshot)
	showCard func()
}

func NewOrchestrator(settings *SettingsStore, manager *auth.Manager, client *api.Client, t *tray.Tray) *Orchestrator {
	return &Orchestrator{
		settings: settings,
		manager:  manager,
		client:   client,
		buddy:    service.NewBuddyService(),
		tray:     t,
		snapshot: newErrorSnapshot(""),
		stopCh:   make(chan struct{}),
	}
}

func (o *Orchestrator) SetEmit(fn func(UsageSnapshot)) { o.emit = fn }
func (o *Orchestrator) SetShowCard(fn func())          { o.showCard = fn }

func (o *Orchestrator) Snapshot() UsageSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.snapshot
}

func (o *Orchestrator) SetSnapshot(s UsageSnapshot) {
	o.mu.Lock()
	o.snapshot = s
	o.mu.Unlock()
	if o.emit != nil {
		o.emit(s)
	}
}

// Start 启动首次刷新与定时器。
func (o *Orchestrator) Start() {
	o.Refresh()
	o.wg.Add(1)
	go o.timerLoop()
}

func (o *Orchestrator) timerLoop() {
	defer o.wg.Done()
	var ticker *time.Ticker
	var ch <-chan time.Time
	lastMinutes := ^uint64(0)
	for {
		select {
		case <-o.stopCh:
			if ticker != nil {
				ticker.Stop()
			}
			return
		case <-ch:
			o.Refresh()
		default:
			mins := o.settings.Get().RefreshIntervalMinutes
			if mins != lastMinutes {
				if ticker != nil {
					ticker.Stop()
				}
				if mins > 0 {
					ticker = time.NewTicker(time.Duration(mins) * time.Minute)
					ch = ticker.C
				} else {
					ticker = nil
					ch = nil
				}
				lastMinutes = mins
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Refresh 执行一次完整刷新（签到 + 喵喵 → 用量）。
func (o *Orchestrator) Refresh() {
	if !o.updating.CompareAndSwap(false, true) {
		return
	}
	defer o.updating.Store(false)

	if o.tray != nil {
		_ = o.tray.SetIcon("…", tray.IconRefreshing)
	}

	cfg := o.settings.Get()
	authInfo, err := o.manager.Current(cfg.AccessToken)
	if err != nil {
		slog.Warn("auth failed", "error", err)
	}
	slog.Info("auth resolved", "mode", func() string { if authInfo != nil { return string(authInfo.Mode) }; return "nil" }(), "hasToken", authInfo != nil && authInfo.Token != "")
	if authInfo == nil || authInfo.Mode == auth.ModeNone {
		slog.Warn("no credentials, showing warning icon")
		snap := newErrorSnapshot("未找到登录凭据，请在托盘菜单选择修复")
		o.SetSnapshot(snap)
		if o.tray != nil {
			_ = o.tray.SetIcon("!", tray.IconWarning)
			o.tray.SetTooltip("CodeBuddy Usage - 未找到登录凭据")
		}
		return
	}

	note := authInfo.Note
	var checkin service.CheckinResult
	var buddy *service.BuddyView
	var usage service.UsageResult

	// 签到与喵喵并行，完成后再拉用量。
	var wg sync.WaitGroup
	if cfg.AutoCheckin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var e error
			checkin, e = service.EnsureCheckin(o.client)
			if e != nil {
				note = describeAuthError(e)
			}
		}()
	}
	if cfg.BuddyTravel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buddy = o.buddy.EnsureBuddy(o.client, true)
		}()
	}
	wg.Wait()

	usage, err = service.FetchUsage(o.client)
	if err != nil {
		slog.Warn("usage fetch failed", "error", err)
	} else {
		slog.Info("usage fetched", "remain", usage.Remain, "total", usage.Total)
	}
	if err != nil {
		note = describeAuthError(err)
		if isAuthErr(err) {
			o.manager.Invalidate(true)
			o.SetSnapshot(newErrorSnapshot("登录已过期，请重新读取或粘贴 Token"))
			if o.tray != nil {
				_ = o.tray.SetIcon("!", tray.IconWarning)
			}
			return
		}
	}

	snap := snapshotFrom(authInfo, usage, checkin, buddy, note)
	o.SetSnapshot(snap)

	if o.tray != nil {
		label := tray.FormatCompact(snap.Remain)
		if err := o.tray.SetIcon(label, tray.IconNormal); err != nil {
			slog.Warn("set tray icon failed", "error", err)
		} else {
			_, _, _, _, ok := o.tray.GetIconRect()
			slog.Info("tray icon ready", "label", label, "rectAvailable", ok)
		}
		o.tray.SetTooltip("CodeBuddy Usage: " + label + " 积分")
		o.notifyResults(snap)
	}
}

func isAuthErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return s == auth.ErrNoCredentials.Error() || s == auth.ErrAuthExpired.Error() ||
		contains(s, "NO_CREDENTIALS") || contains(s, "AUTH_EXPIRED")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func (o *Orchestrator) notifyResults(snap UsageSnapshot) {
	if snap.Checkin.State == "claimed" && snap.Checkin.FreshlyClaimed {
		if snap.Checkin.Credit > 0 {
			o.tray.ShowBalloon("CodeBuddy Usage", "每日签到完成，+" + formatNum(snap.Checkin.Credit) + " 积分")
		} else {
			o.tray.ShowBalloon("CodeBuddy Usage", "每日签到完成")
		}
	}
	if b := snap.Buddy; b != nil {
		if b.FreshlyClaimed {
			msg := "喵喵旅行积分已领取"
			if b.ClaimCredit != nil && *b.ClaimCredit > 0 {
				msg = "喵喵旅行积分已领取，+" + formatNum(float64(*b.ClaimCredit))
			}
			o.tray.ShowBalloon("CodeBuddy Usage", msg)
		}
		if b.FreshlyDeparted {
			o.tray.ShowBalloon("CodeBuddy Usage", "喵喵已出发，约 " + formatNum(b.DurationHours) + " 小时")
		}
	}
}

func formatNum(v float64) string {
	s := time.Duration(0).String()
	_ = s
	if v == float64(int64(v)) {
		return intToString(int64(v))
	}
	return floatToString(v, 2)
}

func intToString(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [24]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func floatToString(v float64, precision int) string {
	scale := 1.0
	for i := 0; i < precision; i++ {
		scale *= 10
	}
	n := int64(v * scale)
	whole := n / int64(scale)
	frac := n % int64(scale)
	if frac < 0 {
		frac = -frac
	}
	if frac == 0 {
		return intToString(whole)
	}
	fracStr := intToString(frac)
	for len(fracStr) < precision {
		fracStr = "0" + fracStr
	}
	return intToString(whole) + "." + fracStr
}

// Claim 手动领积分。
func (o *Orchestrator) Claim() (map[string]interface{}, error) {
	credit, claimed, err := o.buddy.ManualClaim(o.client)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, nil
	}
	if !claimed && credit <= 0 {
		return map[string]interface{}{"credit": 0, "claimed": false}, nil
	}
	o.Refresh()
	return map[string]interface{}{"credit": credit, "claimed": true}, nil
}

// Depart 手动派出。
func (o *Orchestrator) Depart() (map[string]interface{}, error) {
	hours, err := o.buddy.ManualDepart(o.client)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, nil
	}
	o.Refresh()
	return map[string]interface{}{"hours": hours}, nil
}

// FixAuth 解除封锁并重读凭据。
func (o *Orchestrator) FixAuth() {
	o.manager.ResetAutoReadBlock()
	o.manager.Invalidate(true)
	o.Refresh()
}

func (o *Orchestrator) Stop() {
	o.stopOnce.Do(func() { close(o.stopCh) })
	o.wg.Wait()
}
