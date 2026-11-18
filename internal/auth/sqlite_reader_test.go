package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadValueByKeyMatch_LocalVscdb(t *testing.T) {
	// 该测试依赖本机 VS Code/CodeBuddy 登录态，默认跳过；
	// 设置 CB_LIVE_CHECK=1 启用，用于验证 WAL 叠加与溢出页解析。
	if os.Getenv("CB_LIVE_CHECK") != "1" {
		t.Skip("set CB_LIVE_CHECK=1 to test against local vscdb")
	}
	found := false
	for _, product := range []string{"Code", "CodeBuddy CN", "CodeBuddy"} {
		path := filepath.Join(os.Getenv("APPDATA"), product, "User", "globalStorage", "state.vscdb")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		value, ok := ReadValueByKeyMatch(path, func(k string) bool {
			return strings.Contains(k, "accessToken") || strings.Contains(k, "telemetry.firstSessionDate")
		})
		if ok && value != "" {
			found = true
			t.Logf("product=%s read value length=%d", product, len(value))
			break
		}
	}
	if !found {
		t.Fatal("no value could be read from any local vscdb")
	}
}

func TestReadValueByKeyMatch_MissingFile(t *testing.T) {
	_, ok := ReadValueByKeyMatch(filepath.Join(os.TempDir(), "definitely-missing-vscdb-xyz"), func(string) bool { return true })
	if ok {
		t.Fatal("missing file should return false")
	}
}
