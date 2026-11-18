package auth

import "errors"

// AuthMode 表示当前生效的凭据来源。
type AuthMode string

const (
	ModeTokenAuto   AuthMode = "token-auto"
	ModeTokenManual AuthMode = "token-manual"
	ModeNone        AuthMode = "none"
)

// Profile 是从 CodeBuddy 会话明文或 JWT 中提取的账号信息。
type Profile struct {
	Nickname        string `json:"nickname,omitempty"`
	AvatarURL       string `json:"avatarUrl,omitempty"`
	AvatarExpiresAt int64  `json:"avatarExpiresAt,omitempty"` // 毫秒时间戳
}

// AuthInfo 是当前生效的鉴权状态。
type AuthInfo struct {
	Mode      AuthMode
	Token     string
	ExpiresAt int64 // 毫秒时间戳，0 表示未知
	Profile   Profile
	Note      string
}

// CachedToken 是持久化缓存的内容。
type CachedToken struct {
	Token     string  `json:"token"`
	ExpiresAt int64   `json:"expiresAt"`
	Profile   Profile `json:"profile"`
}

// CredentialError 是语义化凭据错误。
type CredentialError struct {
	Code    string
	Message string
}

func (e *CredentialError) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

var (
	ErrNoCredentials = &CredentialError{Code: "NO_CREDENTIALS", Message: "未找到登录凭据"}
	ErrAuthExpired   = &CredentialError{Code: "AUTH_EXPIRED", Message: "登录已过期"}
)

// TokenStore 抽象 token 持久化缓存。
type TokenStore interface {
	Get() (*CachedToken, error)
	Set(*CachedToken) error
	Clear() error
}

var errNotFound = errors.New("not found")
