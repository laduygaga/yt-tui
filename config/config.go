package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DataDir       string
	Player        string
	MaxResults    int
	ChromeProfile string
}

func Load() *Config {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	dataDir := os.Getenv("YT_TUI_DIR")
	if dataDir == "" {
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig != "" {
			dataDir = filepath.Join(xdgConfig, "yt-tui")
		} else {
			dataDir = filepath.Join(home, ".yt-tui")
		}
	}

	os.MkdirAll(dataDir, 0755)

	return &Config{
		DataDir:       dataDir,
		Player:        "mpv",
		MaxResults:    20,
		ChromeProfile: os.Getenv("YT_TUI_CHROME_PROFILE"),
	}
}
