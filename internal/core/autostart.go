package core

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// SetAutostart 增删开机自启注册表项。
func SetAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err := key.DeleteValue("CodeBuddyUsage")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	return key.SetStringValue("CodeBuddyUsage", `"`+exePath()+`"`)
}

func IsAutostart() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	v, _, err := key.GetStringValue("CodeBuddyUsage")
	if err != nil {
		return false
	}
	exe := exePath()
	return v == exe || v == `"`+exe+`"`
}

func ConfigDir() string {
	return AppDataDir()
}
