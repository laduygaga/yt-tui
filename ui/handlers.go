package ui

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"yt-tui/youtube"
)

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmQuit && msg.String() == "q" {
		m.player.Stop()
		return m, tea.Quit
	}

	if m.confirmQuit {
		m.confirmQuit = false
		m.statusMsg = ""
	}

	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q":
			m.showHelp = false
		case "p":
			if m.player.IsPlaying() {
				m.player.TogglePause()
			}
		}
		return m, nil
	}

	if m.showTranscriptView {
		switch msg.String() {
		case "t", "esc", "q":
			m.showTranscriptView = false
			return m, nil
		case "p":
			if m.player.IsPlaying() {
				m.player.TogglePause()
			}
			return m, nil
		case "j", "down":
			if m.transcriptScrollIdx < len(m.transcript.Lines)-1 {
				m.transcriptScrollIdx++
			}
			return m, nil
		case "k", "up":
			if m.transcriptScrollIdx > 0 {
				m.transcriptScrollIdx--
			}
			return m, nil
		case "g":
			m.transcriptScrollIdx = 0
			return m, nil
		case "G":
			m.transcriptScrollIdx = len(m.transcript.Lines) - 1
			if m.transcriptScrollIdx < 0 {
				m.transcriptScrollIdx = 0
			}
			return m, nil
		}
		return m, nil
	}

	if m.showPlaylists {
		return m.handlePlaylistKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.player.Stop()
		return m, tea.Quit
	case "ctrl+l":
		if m.view == "history" {
			m.store.ClearHistory()
			m.statusMsg = "History cleared"
			return m, tea.Batch(
				m.loadHistory(),
				tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
					return clearStatusMsg{}
				}),
			)
		}
		if m.view == "playlist" && m.currentPlaylist == "favorit" {
			m.store.ClearPlaylist("favorit")
			m.videos = []youtube.Video{}
			m.statusMsg = "Favorit cleared"
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "esc", "q":
		if m.confirmQuit {
			m.confirmQuit = false
			return m, nil
		}
		if m.view != "main" {
			m.view = "main"
			m.videos = m.mainVideos
			m.selectedIdx = m.mainSelected
			m.scrollIdx = m.mainScroll
			if len(m.videos) == 0 {
				return m, m.searchVideos("trending")
			}
			return m, nil
		}
		if m.mode == "insert" {
			m.mode = "normal"
			m.searchInput.Blur()
			return m, nil
		}
		m.confirmQuit = true
		m.statusMsg = "Press 'q' again to quit, or any other key to cancel"
		return m, tea.Tick(statusTimeoutMed, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	case "?":
		m.showHelp = true
		return m, nil
	}

	if m.mode == "insert" {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		if msg.Type == tea.KeyEnter {
			query := m.searchInput.Value()
			if query != "" {
				m.mode = "normal"
				m.searchInput.Blur()
				return m, m.searchVideos(query)
			}
		}
		return m, cmd
	}

	switch msg.String() {
	case "i", "/":
		m.mode = "insert"
		m.searchInput.Focus()
		return m, nil
	case "j", "down":
		if m.selectedIdx < len(m.videos)-1 {
			m.selectedIdx++
			m.fixScroll()
		}
		return m, nil
	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.fixScroll()
		}
		return m, nil
	case "p":
		if m.player.IsPlaying() {
			m.player.TogglePause()
		} else if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
		return m, nil
	case "h":
		if m.nowPlaying != "" {
			m.player.Seek(-seekSeconds)
		}
		return m, nil
	case "l":
		if m.nowPlaying != "" {
			m.player.Seek(seekSeconds)
		} else if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
		return m, nil
	case "L":
		if m.nowPlaying != "" {
			m.player.SetLoop(!m.player.IsLooping())
			if m.player.IsLooping() {
				m.statusMsg = "Loop: ON"
			} else {
				m.statusMsg = "Loop: OFF"
			}
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "s":
		m.player.Stop()
		m.nowPlaying = ""
		return m, nil
	case "d":
		if m.lastKey == "d" {
			m.lastKey = ""
			if m.view == "history" && len(m.videos) > 0 {
				m.store.RemoveFromHistory(m.selectedIdx)
				return m, m.loadHistory()
			}
		} else {
			m.lastKey = "d"
		}
		return m, nil
	case "g":
		m.lastKey = "g"
		m.selectedIdx = 0
		m.scrollIdx = 0
		m.mode = "normal"
		return m, nil
	case "G":
		m.selectedIdx = len(m.videos) - 1
		m.fixScroll()
		return m, nil
	case "enter", " ":
		if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
	case "*":
		if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			added, err := m.store.ToggleFavorite(video)
			if err == nil {
				if added {
					m.statusMsg = "Added to favorit"
				} else {
					m.statusMsg = "Removed from favorit"
				}

				if m.view == "playlist" && m.currentPlaylist == "favorit" {
					videos, _ := m.store.GetPlaylist("favorit")
					m.videos = videos
					if m.selectedIdx >= len(m.videos) && len(m.videos) > 0 {
						m.selectedIdx = len(m.videos) - 1
					}
				}

				return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
					return clearStatusMsg{}
				})
			}
		}
		return m, nil
	case "H":
		if m.view == "history" {
			m.view = "main"
			m.videos = m.mainVideos
			m.selectedIdx = m.mainSelected
			m.scrollIdx = m.mainScroll
			if len(m.videos) == 0 {
				return m, m.searchVideos("trending")
			}
			return m, nil
		}
		if m.view == "main" {
			m.mainSelected = m.selectedIdx
			m.mainScroll = m.scrollIdx
		}
		m.view = "history"
		m.videos = []youtube.Video{}
		m.loading = true
		m.loadingText = "Loading history..."
		return m, m.loadHistory()
	case "P":
		if m.showPlaylists {
			m.showPlaylists = false
			if m.view == "main" {
				m.selectedIdx = m.mainSelected
				m.scrollIdx = m.mainScroll
			} else {
				m.selectedIdx = 0
				m.scrollIdx = 0
			}
			m.mode = "normal"
			return m, nil
		}
		if m.view == "main" {
			m.mainSelected = m.selectedIdx
			m.mainScroll = m.scrollIdx
		}
		m.showPlaylists = true
		m.selectedIdx = 0
		m.scrollIdx = 0
		playlists, _ := m.store.ListPlaylists()
		m.playlists = playlists
		return m, nil
	case "c":
		m.showSubtitles = !m.showSubtitles
		if m.showSubtitles {
			m.statusMsg = "Subtitles ON"
		} else {
			m.statusMsg = "Subtitles OFF"
		}
		return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	case "t":
		if m.nowPlaying != "" {
			m.showTranscriptView = !m.showTranscriptView
			if m.showTranscriptView {
				m.statusMsg = "Transcript view ON"
			} else {
				m.statusMsg = "Transcript view OFF"
			}
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "[":
		if m.nowPlaying != "" {
			newSpeed := m.player.PlaybackSpeed() - speedStep
			if newSpeed < speedMin {
				newSpeed = speedMin
			}
			m.playbackSpeed = newSpeed
			m.player.SetSpeed(newSpeed)
			m.statusMsg = fmt.Sprintf("Speed: %.2fx", m.playbackSpeed)
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "]":
		if m.nowPlaying != "" {
			newSpeed := m.player.PlaybackSpeed() + speedStep
			if newSpeed > speedMax {
				newSpeed = speedMax
			}
			m.playbackSpeed = newSpeed
			m.player.SetSpeed(newSpeed)
			m.statusMsg = fmt.Sprintf("Speed: %.2fx", m.playbackSpeed)
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "y":
		if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			url := video.URL
			if url == "" {
				url = "https://www.youtube.com/watch?v=" + video.ID
			}
			cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | xclip -selection clipboard 2>/dev/null || echo '%s' | pbcopy 2>/dev/null || echo '%s' | wl-copy 2>/dev/null", url, url, url))
			err := cmd.Run()
			if err == nil {
				m.statusMsg = "URL copied to clipboard"
			} else {
				m.statusMsg = "Failed to copy (install xclip/pbcopy/wl-copy)"
			}
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) handlePlaylistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "p":
		if m.player.IsPlaying() {
			m.player.TogglePause()
		}
		return m, nil
	case "enter":
		if m.selectedIdx < len(m.playlists) {
			playlistName := m.playlists[m.selectedIdx]
			m.currentPlaylist = playlistName
			videos, _ := m.store.GetPlaylist(playlistName)
			m.videos = videos
			m.selectedIdx = 0
			m.scrollIdx = 0
			m.mode = "normal"
			m.showPlaylists = false
			m.view = "playlist"
		}
	case "j", "down":
		if m.selectedIdx < len(m.playlists)-1 {
			m.selectedIdx++
			m.fixScroll()
		}
	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.fixScroll()
		}
	case "h", "q", "esc", "P":
		m.showPlaylists = false
		if m.view == "main" {
			m.selectedIdx = m.mainSelected
			m.scrollIdx = m.mainScroll
		} else {
			m.selectedIdx = 0
			m.scrollIdx = 0
		}
		m.mode = "normal"
	}
	return m, nil
}
