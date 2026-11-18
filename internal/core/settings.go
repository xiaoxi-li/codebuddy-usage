package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings 是用户配置。
type Settings struct {
	AccessToken           string `json:"accessToken"`
	RefreshIntervalMinutes uint64 `json:"refreshIntervalMinutes"`
	AutoCheckin           bool   `json:"autoCheckin"`
	BuddyTravel           bool   `json:"buddyTravel"`
	ApiBase               string `json:"apiBase"`
	Autostart             bool   `json:"autostart"`
}

func DefaultSettings() Settings {
	return Settings{
		RefreshIntervalMinutes: 30,
		AutoCheckin:           true,
		BuddyTravel:           true,
		ApiBase:               "https://www.workbuddy.cn",
	}
}

// SettingsStore 负责配置的 JSON 持久化。
type SettingsStore struct {
	mu   sync.RWMutex
	path string
	data Settings
}

func NewSettingsStore(dir string) *SettingsStore {
	if dir == "" {
		dir = filepath.Join(os.Getenv("APPDATA"), "codebuddy-usage-desktop")
	}
	s := &SettingsStore{path: filepath.Join(dir, "settings.json"), data: DefaultSettings()}
	_ = os.MkdirAll(dir, 0o700)
	if raw, err := os.ReadFile(s.path); err == nil {
		var loaded Settings
		if err := json.Unmarshal(raw, &loaded); err == nil {
			// 用原始 JSON 的 key 存在性区分"未设置"与"显式 false/0"，
			// 保证老配置缺少字段时仍启用自动签到与喵喵旅行。
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			def := DefaultSettings()
			if _, ok := fields["refreshIntervalMinutes"]; !ok {
				loaded.RefreshIntervalMinutes = def.RefreshIntervalMinutes
			}
			if _, ok := fields["autoCheckin"]; !ok {
				loaded.AutoCheckin = def.AutoCheckin
			}
			if _, ok := fields["buddyTravel"]; !ok {
				loaded.BuddyTravel = def.BuddyTravel
			}
			if _, ok := fields["apiBase"]; !ok || loaded.ApiBase == "" {
				loaded.ApiBase = def.ApiBase
			}
			s.data = loaded
		}
	}
	return s
}

func (s *Settings) applyDefaults() {
	if s.ApiBase == "" {
		s.ApiBase = "https://www.workbuddy.cn"
	}
	// refreshIntervalMinutes==0 是合法的"关闭自动刷新"，不能强制改回 30。
}

func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *SettingsStore) Save(v Settings) error {
	v.applyDefaults()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		return err
	}
	s.data = v
	return nil
}

func (s *SettingsStore) Path() string { return s.path }

// AppDataDir 返回应用运行目录（exe 所在目录）。
// 配置、token 缓存与日志都放在这里，便于携带与清理。
func AppDataDir() string {
	exe, err := os.Executable()
	if err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return dir
		}
	}
	if p := os.Getenv("APPDATA"); p != "" {
		return filepath.Join(p, "codebuddy-usage-desktop")
	}
	return "."
}
