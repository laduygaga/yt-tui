package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DataDir    string
	Player     string
	MaxResults int
}

func Load() *Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".yt-tui")

	os.MkdirAll(dataDir, 0755)

	return &Config{
		DataDir:    dataDir,
		Player:     "mpv",
		MaxResults: 20,
	}
}
