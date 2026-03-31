package ui

import "time"

const mpvSocketPath = "/tmp/yt-tui-mpv.sock"

const (
	seekSeconds         = 10.0
	speedStep           = 0.25
	speedMin            = 0.25
	speedMax            = 2.0
	defaultSpeed        = 1.0
	maxHistoryItems     = 100
	subtitleStartOffset = 0.3
	subtitleEndOffset   = 0.1
	uiOffsetBase        = 15
	borderPadding       = 4
	videoItemHeight     = 2
)

var (
	statusTimeoutShort = 2 * time.Second
	statusTimeoutMed   = 3 * time.Second
	statusTimeoutLong  = 4 * time.Second
)
