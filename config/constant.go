package config

import (
	"os"
	"path/filepath"
	"time"
)

const (
	Version            = "25.0701"
	CatalogVersionInfo = "#版本=1.0"
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	defaultFileExtension = ".jpg"

	defaultRetry   = 3
	defaultTimeout = 300 * time.Second
	defaultQuality = 80
	defaultFormat  = "full/full/0/default.jpg"
)

func DefaultUserAgent() string {
	return defaultUserAgent
}

func UserHomeDir() string {
	if os.PathSeparator == '\\' {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}

func BookgetHomeDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(home, "bookget")
		if os.PathSeparator == '\\' {
			configDir = filepath.Join(home, "bookget")
		}
		if err := os.MkdirAll(configDir, 0755); err != nil && !os.IsExist(err) {
			return ""
		}
		return configDir
	}
	return ""
}

func CacheDir() string {
	return filepath.Join(BookgetHomeDir(), "cache")
}
