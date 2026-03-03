package main

import (
	"fmt"
	"os"

	"yt-tui/config"
	"yt-tui/storage"
	"yt-tui/ui"
)

func main() {
	cfg := config.Load()
	store := storage.New(cfg)

	if err := ui.Run(cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "Error running app: %v\n", err)
		os.Exit(1)
	}
}
