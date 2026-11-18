package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

func makeJWT(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return header + "." + body + ".sig"
}

func TestJWTExpiryAndNickname(t *testing.T) {
	exp := time.Now().Add(60 * 24 * time.Hour).Unix()
	token := makeJWT(`{"exp":` + itoa(exp) + `,"nickname":"QIN"}`)
	got := JWTExpiry(token)
	if got != exp*1000 {
		t.Fatalf("expiry = %d, want %d", got, exp*1000)
	}
	if n := JWTNickname(token); n != "QIN" {
		t.Fatalf("nickname = %q, want QIN", n)
	}
}

func TestJWTExpiryInvalid(t *testing.T) {
	if JWTExpiry("not-a-jwt") != 0 {
		t.Fatal("invalid token should have zero expiry")
	}
	if JWTNickname("") != "" {
		t.Fatal("empty token should have empty nickname")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [24]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type memoryStore struct {
	data *CachedToken
}

func (m *memoryStore) Get() (*CachedToken, error) {
	if m.data == nil {
		return nil, errNotFound
	}
	c := *m.data
	return &c, nil
}
func (m *memoryStore) Set(v *CachedToken) error {
	c := *v
	m.data = &c
	return nil
}
func (m *memoryStore) Clear() error { m.data = nil; return nil }

func TestManagerUsesValidCachedToken(t *testing.T) {
	store := &memoryStore{}
	token := makeJWT(`{"exp":` + itoa(time.Now().Add(60*24*time.Hour).Unix()) + `,"nickname":"cached"}`)
	exp := JWTExpiry(token)
	store.data = &CachedToken{Token: token, ExpiresAt: exp}
	mgr := NewManager(store)
	mgr.autoReadBlocked = true // 避免本机真实凭据干扰，强制走缓存
	info, err := mgr.Current("")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode != ModeTokenAuto || info.Token != token {
		t.Fatalf("mode=%s token=%q, want cached token", info.Mode, info.Token)
	}
	if info.Profile.Nickname != "cached" {
		t.Fatalf("nickname=%q, want cached", info.Profile.Nickname)
	}
}
