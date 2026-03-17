# YouTube TUI

A lightweight, keyboard-driven YouTube search and playback tool for the terminal. Built with Go and the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework, using `mpv` for playback and `yt-dlp` for searching.

## Features

- **Search & Discovery**: Search for any content or browse trending videos.
- **Vim-like Navigation**: Fast, keyboard-centric control.
- **Full Playback Control**: Seek (10s), Pause, Resume, and Stop with real-time progress bar synchronization.
- **History Management**: Automatically records watch history with duplicate handling and Vim-style deletion.
- **Favorites**: Toggle videos in your `favorit` playlist with a single key.
- **Local Storage**: All history and playlists are stored as portable JSON files.
- **Robust Navigation**: Instant "back" functionality with state preservation.

## Prerequisites

To use this project, you must have the following installed:

- [Go](https://golang.org/doc/install) (to build)
- [mpv](https://mpv.io/) (for audio/video playback)
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) (for searching and stream URL extraction)

## Installation

```bash
git clone https://github.com/yourusername/yt-tui.git
cd yt-tui
go build -o yt-tui main.go
./yt-tui
```

## Keybindings

### Global Controls
- `?`: Toggle Help menu
- `Ctrl+C`: Force Quit
- `q` / `Esc`: Back to previous view / Normal mode / Quit

### Navigation
- `j` / `Down`: Move selection down
- `k` / `Up`: Move selection up
- `g`: Jump to top
- `G`: Jump to bottom
- `Enter` / `Space` / `l`: Play selected video

### Search
- `/` or `i`: Enter search mode
- `Enter`: Submit search (while in search mode)

### Playback
- `p`: Pause / Resume
- `s`: Stop current video
- `h`: Seek backward 10s
- `l`: Seek forward 10s (when video is playing)

### History & Playlists
- `H`: View Watch History
- `P`: View Playlists menu
- `*`: Toggle current video in `favorit` playlist
- `dd`: Delete item from current list (History/Playlist)
- `Ctrl+L`: Clear current list (History/Favorit)

## Data Storage

By default, data is stored in `~/.yt-tui/`. The application respects the following environment variables for data location:

1. `YT_TUI_DIR`
2. `XDG_CONFIG_HOME/yt-tui`
3. `~/.yt-tui` (fallback)

## License

MIT
