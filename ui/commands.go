package ui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"yt-tui/config"
	"yt-tui/storage"
	"yt-tui/youtube"
)

type searchResultMsg struct {
	videos []youtube.Video
	err    error
}

type playResultMsg struct {
	url   string
	video youtube.Video
	err   error
}

type transcriptResultMsg struct {
	transcript *youtube.Transcript
	err        error
}

func (m *Model) searchVideos(query string) tea.Cmd {
	m.view = "main"
	m.loading = true
	m.loadingText = "Searching " + query + "..."
	m.selectedIdx = 0
	m.scrollIdx = 0
	m.mainSelected = 0
	m.mainScroll = 0
	m.videos = []youtube.Video{}
	return func() tea.Msg {
		videos, err := youtube.Search(query, m.cfg.MaxResults)
		return searchResultMsg{videos: videos, err: err}
	}
}

func (m *Model) playVideo(video youtube.Video) tea.Cmd {
	return func() tea.Msg {
		url := video.URL
		if url == "" {
			url = "https://www.youtube.com/watch?v=" + video.ID
		}

		streamURL, err := youtube.GetStreamURL(url, m.cfg.ChromeProfile)
		if err != nil {
			return playResultMsg{url: "", video: video, err: err}
		}

		return playResultMsg{url: streamURL, video: video, err: nil}
	}
}

func (m *Model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		history, err := m.store.GetHistory()
		if err != nil {
			return searchResultMsg{videos: nil, err: err}
		}
		if len(history) == 0 {
			path := filepath.Join(m.cfg.DataDir, "history.json")
			info, _ := os.Stat(path)
			if info != nil && info.Size() > 10 {
				return searchResultMsg{videos: nil, err: fmt.Errorf("loaded 0 items from %dB file", info.Size())}
			}
			return searchResultMsg{videos: []youtube.Video{}, err: nil}
		}
		var videos []youtube.Video
		for _, h := range history {
			videos = append(videos, h.Video)
		}
		return searchResultMsg{videos: videos, err: nil}
	}
}

func (m *Model) loadTranscript(videoID string) tea.Cmd {
	return func() tea.Msg {
		cached, err := m.store.GetTranscript(videoID)
		if err == nil && cached != nil {
			return transcriptResultMsg{transcript: cached, err: nil}
		}

		transcript, err := youtube.GetTranscript(videoID)
		if err != nil {
			return transcriptResultMsg{transcript: nil, err: err}
		}

		return transcriptResultMsg{transcript: transcript, err: nil}
	}
}

func Run(cfg *config.Config, store *storage.Storage) error {
	m := New(cfg, store)
	p := tea.NewProgram(m, tea.WithOutput(nil))
	m.program = p
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
