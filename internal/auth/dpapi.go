package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32DLL               = windows.NewLazySystemDLL("crypt32.dll")
	procCryptUnprotectData   = crypt32DLL.NewProc("CryptUnprotectData")
	procCryptProtectData     = crypt32DLL.NewProc("CryptProtectData")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(data []byte) dataBlob {
	b := dataBlob{cbData: uint32(len(data))}
	if len(data) > 0 {
		b.pbData = &data[0]
	}
	return b
}

func (b *dataBlob) bytes() []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	return unsafe.Slice(b.pbData, int(b.cbData))
}

func cryptUnprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty protected data")
	}
	in := newBlob(data)
	var out dataBlob
	ret, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return nil, callErr
		}
		return nil, errors.New("CryptUnprotectData failed")
	}
	result := append([]byte(nil), out.bytes()...)
	if out.pbData != nil {
		windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	}
	return result, nil
}

func cryptProtect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	in := newBlob(data)
	var out dataBlob
	ret, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return nil, callErr
		}
		return nil, errors.New("CryptProtectData failed")
	}
	result := append([]byte(nil), out.bytes()...)
	if out.pbData != nil {
		windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	}
	return result, nil
}

// ReadWindowsKey 从 Local State 解出 32 字节 safeStorage AES key。
func ReadWindowsKey(localStatePath string) ([]byte, error) {
	raw, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, err
	}
	var state struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.OSCrypt.EncryptedKey == "" {
		return nil, errors.New("os_crypt.encrypted_key missing")
	}
	protected, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(protected, []byte("DPAPI")) {
		protected = protected[5:]
	}
	key, err := cryptUnprotect(protected)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("unexpected key length")
	}
	return key, nil
}

// AppDataDir 返回本机 Roaming AppData 路径。
func AppDataDir() string {
	if p := os.Getenv("APPDATA"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming")
}

// FileTokenStore 用 DPAPI 加密后把 token 缓存落盘。
type FileTokenStore struct {
	Path string
}

func (s *FileTokenStore) Get() (*CachedToken, error) {
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNotFound
		}
		return nil, err
	}
	plain, err := cryptUnprotect(raw)
	if err != nil {
		return nil, err
	}
	var cached CachedToken
	if err := json.Unmarshal(plain, &cached); err != nil {
		return nil, err
	}
	if cached.Token == "" {
		return nil, errNotFound
	}
	return &cached, nil
}

func (s *FileTokenStore) Set(v *CachedToken) error {
	if v == nil || v.Token == "" {
		return s.Clear()
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	protected, err := cryptProtect(plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.Path, protected, 0o600)
}

func (s *FileTokenStore) Clear() error {
	err := os.Remove(s.Path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ProtectString / UnprotectString 供设置页手动 token 加密保存使用。
func ProtectString(s string) (string, error) {
	b, err := cryptProtect([]byte(s))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func UnprotectString(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	plain, err := cryptUnprotect(b)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
