package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"regexp"
	"strconv"
)

// DecryptV10GCM 解密 Electron safeStorage 的 v10 AES-256-GCM 密文。
// 布局：'v10' (3) + nonce (12) + ciphertext + tag (16)
func DecryptV10GCM(enc, key []byte) ([]byte, error) {
	if len(enc) <= 3+12+16 {
		return nil, errors.New("ciphertext too short")
	}
	if !bytes.Equal(enc[:3], []byte("v10")) {
		return nil, errors.New("unsupported prefix")
	}
	nonce := enc[3:15]
	tag := enc[len(enc)-16:]
	data := enc[15 : len(enc)-16]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := make([]byte, 0, len(data)+len(tag))
	sealed = append(sealed, data...)
	sealed = append(sealed, tag...)
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

var (
	accessTokenRe = regexp.MustCompile(`"accessToken":"([^"]+)"`)
	nicknameRe    = regexp.MustCompile(`"nickname":"([^"]*)"`)
	avatarRe      = regexp.MustCompile(`"avatarUrl":"([^"]*)"`)
	signTimeRe    = regexp.MustCompile(`(?:^|&)q-sign-time=(\d+)[;%3B]*(\d+)`)
)

// ExtractAccessToken 从解密后的会话明文中提取 accessToken。
func ExtractAccessToken(plain []byte) string {
	m := accessTokenRe.FindSubmatch(plain)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// ExtractProfile 提取昵称与头像 URL 及其签名到期时间。
func ExtractProfile(plain []byte) Profile {
	nickname := ""
	if m := nicknameRe.FindSubmatch(plain); len(m) >= 2 {
		nickname = string(m[1])
	}
	avatar := ""
	if m := avatarRe.FindSubmatch(plain); len(m) >= 2 {
		avatar = string(m[1])
	}
	if nickname == "" && avatar == "" {
		return Profile{}
	}
	return Profile{Nickname: nickname, AvatarURL: avatar, AvatarExpiresAt: avatarExpiry(avatar)}
}

func avatarExpiry(url string) int64 {
	idx := bytes.IndexByte([]byte(url), '?')
	if idx < 0 {
		return 0
	}
	query := url[idx+1:]
	if decoded, err := strconv.Unquote("\"" + query + "\""); err == nil {
		query = decoded
	}
	m := signTimeRe.FindStringSubmatch(query)
	if len(m) < 3 {
		return 0
	}
	end, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || end <= 0 {
		return 0
	}
	return end * 1000
}
