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
	playStartTime       time.Time
	playAttempt         int
	cachedView          string
	viewVersion         uint64
	formattedDur        []string
	formattedViews      []string
	progressWidthSet    int
	playStateSnap       player.State
	cachedWidth         int
	cachedTitles        []string
	cachedDesc          []string
}

type syncTimeMsg struct {
	Current float64
	Total   float64
}
type clearStatusMsg struct{}
type songEndedMsg struct{}
type playbackErrorMsg struct {
	video   youtube.Video
	attempt int
}

func (m *Model) tickProgress() tea.Cmd {
	if !m.player.IsPlaying() {
		return nil
	}
	ver := m.player.SnapshotVersion()
	return tea.Tick(progressTickInterval, func(t time.Time) tea.Msg {
		if m.player.SnapshotVersion() == ver && !m.player.IsPlaying() {
			return nil
		}
		state := m.player.GetState()
		return syncTimeMsg{
			Current: state.CurrentTime,
			Total:   state.TotalTime,
		}
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
	case syncTimeMsg:
		if m.shouldApplySyncTime(msg) {
			m.applySyncTime(msg)
		}
		return m, m.tickProgress()
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case clearStatusMsg:
		m.statusMsg = ""
		m.confirmQuit = false
		return m, nil
	case searchResultMsg:
		m.loading = false
		m.setVideos(msg.videos)
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
		if m.playStartTime.IsZero() {
			m.playStartTime = time.Now()
		}
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
	case songEndedMsg:
		quickFail := !m.playStartTime.IsZero() && time.Since(m.playStartTime) < 5*time.Second && m.currentTime < 2

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

		m.player.Stop()
		if m.nowPlaying != "" {
			if quickFail && m.playAttempt < 3 {
				m.playAttempt++
				m.playStartTime = time.Time{}
				if m.selectedIdx < len(m.videos) {
					video := m.videos[m.selectedIdx]
					m.loading = true
					m.loadingText = "Retrying playback..."
					return m, m.playVideo(video)
				}
			}
			m.statusMsg = "Playback ended"
			m.playAttempt = 0
			m.playStartTime = time.Time{}
			return m, tea.Tick(statusTimeoutShort, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		m.playAttempt = 0
		m.playStartTime = time.Time{}
		return m, nil
	}
	return m, nil
}

func (m *Model) setVideos(videos []youtube.Video) {
	m.videos = videos
	m.formattedDur = make([]string, len(videos))
	m.formattedViews = make([]string, len(videos))
	for i, v := range videos {
		m.formattedDur[i] = formatDuration(v.Duration)
		m.formattedViews[i] = formatViews(v.Views)
	}
	m.cachedTitles = nil
	m.cachedDesc = nil
}

func (m *Model) ensureTruncationCache() {
	width := m.width - 10
	if width < 10 {
		width = 10
	}
	if m.cachedWidth == width && len(m.cachedTitles) == len(m.videos) {
		return
	}
	m.cachedWidth = width
	m.cachedTitles = make([]string, len(m.videos))
	m.cachedDesc = make([]string, len(m.videos))
	for i, v := range m.videos {
		m.cachedTitles[i] = truncate(v.Title, width)
		m.cachedDesc[i] = truncate(v.Description, width*3)
	}
}

func (m *Model) shouldApplySyncTime(msg syncTimeMsg) bool {
	current := msg.Current
	if msg.Total > 0 && current > msg.Total {
		current = msg.Total
	}

	if msg.Total > 0 && (m.totalTime <= 0 || hasSignificantTimeDelta(msg.Total, m.totalTime)) {
		return true
	}
	if m.totalTime > 0 && current >= m.totalTime && m.currentTime < m.totalTime {
		return true
	}
	return hasSignificantTimeDelta(current, m.currentTime)
}

func (m *Model) applySyncTime(msg syncTimeMsg) {
	m.currentTime = msg.Current
	if msg.Total > 0 {
		m.totalTime = msg.Total
	}
	if m.totalTime > 0 && m.currentTime > m.totalTime {
		m.currentTime = m.totalTime
	}
}

func hasSignificantTimeDelta(a, b float64) bool {
	if a > b {
		return a-b >= syncTimeMinDelta
	}
	return b-a >= syncTimeMinDelta
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

	err := m.player.Start(url, m.cfg.Player, func() {
		if m.program != nil && !m.player.IsLooping() {
			m.program.Send(songEndedMsg{})
		}
	})

	if err != nil {
		return func() tea.Msg {
			return playResultMsg{err: err}
		}
	}

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
