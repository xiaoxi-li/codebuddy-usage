package auth

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	secretKey      = "Tencent-Cloud.coding-copilot.new.accessToken"
	cacheTTL       = 5 * time.Minute
	refreshMargin  = 24 * time.Hour
	profileMargin  = 48 * time.Hour
	profileRetry   = 12 * time.Hour
)

var productNames = []string{
	"Code",
	"Code - Insiders",
	"CodeBuddy",
	"CodeBuddy CN",
	"Trae",
	"Trae CN",
	"Cursor",
	"Kiro",
	"Qoder",
}

type hostCandidate struct {
	label          string
	dbPath         string
	localStatePath string
}

func hostCandidates() []hostCandidate {
	appData := AppDataDir()
	out := make([]hostCandidate, 0, len(productNames))
	for _, name := range productNames {
		out = append(out, hostCandidate{
			label:          name,
			dbPath:         filepath.Join(appData, name, "User", "globalStorage", "state.vscdb"),
			localStatePath: filepath.Join(appData, name, "Local State"),
		})
	}
	return out
}

// Manager 实现持久化缓存 → 自动读取 → 手动 Token 的三级回退。
type Manager struct {
	mu              sync.Mutex
	cache           *AuthInfo
	cacheAt         time.Time
	store           TokenStore
	autoReadBlocked bool
	lastAutoNote    string
	profileRefreshAt time.Time
}

func NewManager(store TokenStore) *Manager {
	return &Manager{store: store}
}

// Invalidate 清空鉴权缓存。allowCredentialRetry=true 时解除自动读取封锁。
func (m *Manager) Invalidate(allowCredentialRetry bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = nil
	m.cacheAt = time.Time{}
	if allowCredentialRetry {
		m.autoReadBlocked = false
		m.lastAutoNote = ""
	}
	if m.store != nil {
		_ = m.store.Clear()
	}
}

// ResetAutoReadBlock 解除本次会话的自动读取封锁。
func (m *Manager) ResetAutoReadBlock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoReadBlocked = false
	m.lastAutoNote = ""
}

func avatarUsable(p Profile, now time.Time) bool {
	if p.AvatarURL == "" {
		return false
	}
	if p.AvatarExpiresAt == 0 {
		return true
	}
	return p.AvatarExpiresAt > now.UnixMilli()
}

func profileStale(p Profile, now time.Time) bool {
	if p.AvatarURL == "" || p.AvatarExpiresAt == 0 {
		return false
	}
	return time.Duration(p.AvatarExpiresAt-now.UnixMilli())*time.Millisecond < profileMargin
}

func (m *Manager) refreshProfileLocked(state *AuthInfo, now time.Time) {
	if m.autoReadBlocked || now.Sub(m.profileRefreshAt) < profileRetry {
		return
	}
	m.profileRefreshAt = now
	auto, note := m.readAutoTokenLocked()
	if auto == nil || auto.Profile.AvatarURL == "" {
		if note != "" {
			m.lastAutoNote = note
		}
		return
	}
	state.Profile = auto.Profile
	if m.store != nil && state.Token != "" {
		_ = m.store.Set(&CachedToken{
			Token: state.Token, ExpiresAt: state.ExpiresAt, Profile: state.Profile,
		})
	}
}

// Current 返回当前生效的鉴权状态。
func (m *Manager) Current(manualToken string) (*AuthInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()

	if m.cache != nil && now.Sub(m.cacheAt) < cacheTTL {
		if !avatarUsable(m.cache.Profile, now) || profileStale(m.cache.Profile, now) {
			m.refreshProfileLocked(m.cache, now)
		}
		copy := *m.cache
		return &copy, nil
	}

	manualToken = strings.TrimSpace(manualToken)

	// 1. 持久化缓存
	if m.store != nil {
		cached, err := m.store.Get()
		if err == nil && cached != nil && cached.Token != "" {
			expiresAt := cached.ExpiresAt
			if expiresAt == 0 {
				expiresAt = JWTExpiry(cached.Token)
			}
			if expiresAt == 0 || expiresAt > now.UnixMilli()+refreshMargin.Milliseconds() {
				profile := cached.Profile
				if profile.Nickname == "" {
					profile.Nickname = JWTNickname(cached.Token)
				}
				m.cache = &AuthInfo{
					Mode: ModeTokenAuto, Token: cached.Token,
					ExpiresAt: expiresAt, Profile: profile,
				}
				if !avatarUsable(profile, now) {
					m.refreshProfileLocked(m.cache, now)
				}
				m.cacheAt = now
				copy := *m.cache
				return &copy, nil
			}
		}
	}

	// 2. 自动读取
	if !m.autoReadBlocked {
		auto, note := m.readAutoTokenLocked()
		if auto != nil && auto.Token != "" {
			expiresAt := JWTExpiry(auto.Token)
			if expiresAt == 0 || expiresAt > now.UnixMilli() {
				profile := auto.Profile
				if profile.Nickname == "" {
					profile.Nickname = JWTNickname(auto.Token)
				}
				if m.store != nil {
					_ = m.store.Set(&CachedToken{Token: auto.Token, ExpiresAt: expiresAt, Profile: profile})
				}
				m.cache = &AuthInfo{
					Mode: ModeTokenAuto, Token: auto.Token,
					ExpiresAt: expiresAt, Profile: profile,
				}
				m.cacheAt = now
				copy := *m.cache
				return &copy, nil
			}
			m.lastAutoNote = "auto token expired"
		} else {
			m.lastAutoNote = note
		}
		if auto == nil || auto.Token == "" {
			m.autoReadBlocked = true
		}
	}

	// 3. 手动 Token
	if manualToken != "" {
		expiresAt := JWTExpiry(manualToken)
		if expiresAt == 0 || expiresAt > now.UnixMilli() {
			m.cache = &AuthInfo{
				Mode: ModeTokenManual, Token: manualToken, ExpiresAt: expiresAt,
				Note: m.lastAutoNote, Profile: Profile{Nickname: JWTNickname(manualToken)},
			}
			m.cacheAt = now
			copy := *m.cache
			return &copy, nil
		}
		m.lastAutoNote = "manual token expired"
	}

	m.cache = &AuthInfo{Mode: ModeNone, Note: m.lastAutoNote}
	m.cacheAt = now
	copy := *m.cache
	return &copy, nil
}

// readAutoTokenLocked 遍历所有宿主，读取并解密 CodeBuddy 会话明文。
func (m *Manager) readAutoTokenLocked() (*AuthInfo, string) {
	sawDB := false
	lastError := ""
	for _, host := range hostCandidates() {
		enc, ok := readEncryptedSecret(host.dbPath)
		if !ok || len(enc) == 0 {
			continue
		}
		sawDB = true
		key, err := ReadWindowsKey(host.localStatePath)
		if err != nil {
			lastError = err.Error()
			continue
		}
		plain, err := DecryptV10GCM(enc, key)
		if err != nil {
			lastError = "failed to decrypt CodeBuddy session"
			continue
		}
		token := ExtractAccessToken(plain)
		if token != "" {
			return &AuthInfo{Mode: ModeTokenAuto, Token: token, Profile: ExtractProfile(plain)}, ""
		}
		lastError = "accessToken not found in session"
	}
	if !sawDB {
		return nil, "no CodeBuddy session found"
	}
	if lastError == "" {
		lastError = "failed to read CodeBuddy session"
	}
	return nil, lastError
}

func readEncryptedSecret(dbPath string) ([]byte, bool) {
	raw, ok := ReadValueByKeyMatch(dbPath, func(key string) bool {
		return strings.Contains(key, secretKey)
	})
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var parsed struct {
		Data []int `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || len(parsed.Data) == 0 {
		return nil, false
	}
	out := make([]byte, len(parsed.Data))
	for i, b := range parsed.Data {
		if b < 0 || b > 255 {
			return nil, false
		}
		out[i] = byte(b)
	}
	return out, true
}

// AvatarUsable 供 UI 判断是否展示头像。
func AvatarUsable(p Profile) bool {
	return avatarUsable(p, time.Now())
}
