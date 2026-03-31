package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"yt-tui/config"
	"yt-tui/player"
	"yt-tui/storage"
	"yt-tui/youtube"
)

var (
	black  = lipgloss.Color("#000000")
	cyan   = lipgloss.Color("#199aa6")
	gray   = lipgloss.Color("#696c77")
	green  = lipgloss.Color("#50a14f")
	yellow = lipgloss.Color("#c18401")

	normalStyle    = lipgloss.NewStyle().Foreground(black)
	selectedStyle  = lipgloss.NewStyle().Foreground(black).Background(cyan)
	secondaryStyle = lipgloss.NewStyle().Foreground(gray)
	titleStyle     = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	statusStyle    = lipgloss.NewStyle().Foreground(gray)
)

type Model struct {
	cfg                 *config.Config
	store               *storage.Storage
	videos              []youtube.Video
	selectedIdx         int
	mode                string
	view                string
	loading             bool
	loadingText         string
	statusMsg           string
	nowPlaying          string
	searchInput         textinput.Model
	playlists           []string
	showPlaylists       bool
	showHelp            bool
	width               int
	height              int
	scrollIdx           int
	player              *player.Player
	isPaused            bool
	lastKey             string
	progress            progress.Model
	currentTime         float64
	totalTime           float64
	program             *tea.Program
	mainVideos          []youtube.Video
	mainSelected        int
	mainScroll          int
	currentPlaylist     string
	transcript          *youtube.Transcript
	showSubtitles       bool
	showTranscriptView  bool
	transcriptScrollIdx int
	playbackSpeed       float64
	isLooping           bool
	confirmQuit         bool
}

type progressMsg float64
type syncTimeMsg struct {
	Current float64
	Total   float64
}
type clearStatusMsg struct{}
type songEndedMsg struct{}

func (m *Model) tickProgress() tea.Cmd {
	return tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
		if !m.player.IsPlaying() || m.player.IsPaused() {
			return nil
		}

		return progressMsg(1.0)
	})
}

func New(cfg *config.Config, store *storage.Storage) *Model {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube..."
	ti.Prompt = "Search: "
	ti.TextStyle = lipgloss.NewStyle().Foreground(black)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gray)

	pl := player.New(mpvSocketPath)

	return &Model{
		cfg:           cfg,
		store:         store,
		videos:        []youtube.Video{},
		selectedIdx:   0,
		mode:          "normal",
		view:          "main",
		loading:       false,
		loadingText:   "",
		searchInput:   ti,
		progress:      progress.New(progress.WithScaledGradient("#000000", "#696c77")),
		playbackSpeed: defaultSpeed,
		player:        pl,
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case syncTimeMsg:
		m.currentTime = msg.Current
		if msg.Total > 0 {
			m.totalTime = msg.Total
		}
		if m.currentTime > m.totalTime {
			m.currentTime = m.totalTime
		}
		return m, nil
	case clearStatusMsg:
		m.statusMsg = ""
		m.confirmQuit = false
		return m, nil
	case searchResultMsg:
		m.loading = false
		m.videos = msg.videos
		if m.view == "main" {
			m.mainVideos = msg.videos
		}
		if msg.err != nil {
			m.loadingText = msg.err.Error()
			m.loading = true
			return m, nil
		}
		m.selectedIdx = 0
		m.scrollIdx = 0
		m.mode = "normal"
		return m, nil
	case playResultMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMsg = "Failed to play: " + msg.err.Error()
			return m, tea.Tick(statusTimeoutLong, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		m.nowPlaying = msg.video.Title
		m.isPaused = false
		if msg.url != "" {
			msg.video.URL = msg.url
			m.store.AddToHistory(msg.video)
			m.currentTime = 0
			m.totalTime = parseDuration(msg.video.Duration)
		}
		return m, tea.Batch(m.startPlayer(msg.url, msg.video.Title), m.tickProgress(), m.loadTranscript(msg.video.ID))
	case transcriptResultMsg:
		if msg.err != nil {
			m.statusMsg = "Transcript unavailable: " + msg.err.Error()
			return m, tea.Tick(statusTimeoutLong, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		if msg.transcript != nil {
			m.transcript = msg.transcript
			m.store.SaveTranscript(msg.transcript)
			m.statusMsg = "Transcript loaded"
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case progressMsg:
		if m.player.IsPlaying() && !m.player.IsPaused() {
			m.currentTime += float64(msg)
			if m.currentTime > m.totalTime {
				m.currentTime = m.totalTime
			}
			return m, m.tickProgress()
		}
		return m, nil
	case songEndedMsg:
		if !m.player.IsLooping() && (m.view == "playlist" || m.view == "history") && len(m.videos) > 0 {
			nextIdx := m.selectedIdx + 1
			if nextIdx < len(m.videos) {
				m.selectedIdx = nextIdx
				m.fixScroll()
				video := m.videos[m.selectedIdx]
				m.loading = true
				m.loadingText = "Getting stream..."
				return m, m.playVideo(video)
			}
		}
		if m.currentTime > 1 {
			m.player.Stop()
		}
		return m, nil
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m *Model) fixScroll() {
	itemHeight := videoItemHeight
	if m.showPlaylists {
		itemHeight = 1
	}

	offset := uiOffsetBase
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	maxItems := (m.height - offset) / itemHeight
	if maxItems < 1 {
		maxItems = 1
	}

	if m.selectedIdx < m.scrollIdx {
		m.scrollIdx = m.selectedIdx
	}
	if m.selectedIdx >= m.scrollIdx+maxItems {
		m.scrollIdx = m.selectedIdx - maxItems + 1
	}

	if m.scrollIdx < 0 {
		m.scrollIdx = 0
	}
}

func formatTime(seconds float64) string {
	s := int(seconds)
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func parseDuration(duration string) float64 {
	duration = strings.TrimSpace(duration)
	if duration == "" {
		return 0
	}

	if !strings.Contains(duration, ":") {
		var val float64
		_, err := fmt.Sscanf(duration, "%f", &val)
		if err == nil {
			return val
		}
	}

	parts := strings.Split(duration, ":")
	var seconds float64
	for i, part := range parts {
		var val float64
		_, err := fmt.Sscanf(part, "%f", &val)
		if err != nil {
			continue
		}
		multiplier := 1.0
		pos := len(parts) - 1 - i
		if pos == 1 {
			multiplier = 60.0
		} else if pos == 2 {
			multiplier = 3600.0
		}
		seconds += val * multiplier
	}
	return seconds
}

func (m *Model) startPlayer(url, title string) tea.Cmd {
	m.playbackSpeed = defaultSpeed
	m.isLooping = false

	m.player.Start(url, m.cfg.Player, func() {
		if m.program != nil && !m.player.IsLooping() {
			m.program.Send(songEndedMsg{})
		}
	})

	return nil
}

func truncate(s string, maxLen int) string {
	if maxLen < 5 {
		if len(s) > 0 {
			return "..."
		}
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getTitleSuffix(m *Model) string {
	if m.view == "history" {
		return fmt.Sprintf(" - History (%d)", len(m.videos))
	}
	if m.view == "playlist" {
		return fmt.Sprintf(" - %s (%d)", m.currentPlaylist, len(m.videos))
	}
	return ""
}
