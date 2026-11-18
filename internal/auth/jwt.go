package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

func jwtPayload(token string) map[string]interface{} {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	seg := parts[1]
	seg = strings.ReplaceAll(seg, "-", "+")
	seg = strings.ReplaceAll(seg, "_", "/")
	switch len(seg) % 4 {
	case 2:
		seg += "=="
	case 3:
		seg += "="
	}
	raw, err := base64.StdEncoding.DecodeString(seg)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var payload map[string]interface{}
	if err := dec.Decode(&payload); err != nil {
		return nil
	}
	return payload
}

// JWTExpiry 返回毫秒时间戳；非 JWT 或解析失败返回 0。
func JWTExpiry(token string) int64 {
	p := jwtPayload(token)
	if p == nil {
		return 0
	}
	if exp, ok := p["exp"].(float64); ok {
		return int64(exp * 1000)
	}
	return 0
}

// JWTNickname 从 JWT payload 中提取昵称。
func JWTNickname(token string) string {
	p := jwtPayload(token)
	if p == nil {
		return ""
	}
	if n, ok := p["nickname"].(string); ok {
		return n
	}
	return ""
}
