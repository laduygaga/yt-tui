package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"yt-tui/config"
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
	cfg           *config.Config
	store         *storage.Storage
	videos        []youtube.Video
	selectedIdx   int
	mode          string
	view          string
	loading       bool
	loadingText   string
	nowPlaying    string
	searchInput   textinput.Model
	playlists     []string
	showPlaylists bool
	showHelp      bool
	width         int
	height        int
	scrollIdx     int
	playerCmd     *exec.Cmd
	isPaused      bool
}

func New(cfg *config.Config, store *storage.Storage) *Model {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube..."
	ti.Prompt = "Search: "
	ti.TextStyle = lipgloss.NewStyle().Foreground(black)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(gray)

	return &Model{
		cfg:         cfg,
		store:       store,
		videos:      []youtube.Video{},
		selectedIdx: 0,
		mode:        "normal",
		view:        "main",
		loading:     false,
		loadingText: "",
		searchInput: ti,
	}
}

func (m *Model) Init() tea.Cmd {
	return m.searchVideos("trending")
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case searchResultMsg:
		m.loading = false
		m.videos = msg.videos
		m.selectedIdx = 0
		m.scrollIdx = 0
		m.mode = "normal"
		return m, nil
	case playResultMsg:
		m.loading = false
		if msg.err != nil {
			m.nowPlaying = ""
			return m, nil
		}
		m.nowPlaying = msg.title
		m.isPaused = false
		return m, m.startPlayer(msg.url, msg.title)
	}
	return m, nil
}

func (m *Model) startPlayer(url, title string) tea.Cmd {
	if m.playerCmd != nil && m.playerCmd.Process != nil {
		m.playerCmd.Process.Kill()
	}

	m.playerCmd = exec.Command(m.cfg.Player, "--no-video", "--quiet", url)
	err := m.playerCmd.Start()
	if err != nil {
		m.nowPlaying = "Error: " + err.Error()
	}

	video := youtube.Video{Title: title}
	m.store.AddToHistory(video)

	return nil
}

func (m *Model) stopPlayer() {
	if m.playerCmd != nil && m.playerCmd.Process != nil {
		m.playerCmd.Process.Kill()
		m.playerCmd = nil
		m.nowPlaying = ""
		m.isPaused = false
	}
}

func (m *Model) togglePause() {
	if m.playerCmd != nil && m.playerCmd.Process != nil {
		if m.isPaused {
			m.playerCmd.Process.Signal(syscall.SIGCONT)
			m.isPaused = false
		} else {
			m.playerCmd.Process.Signal(syscall.SIGSTOP)
			m.isPaused = true
		}
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		m.stopPlayer()
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "q":
		if m.showPlaylists {
			m.showPlaylists = false
			return m, nil
		}
		if m.mode == "insert" {
			m.mode = "normal"
			m.searchInput.Blur()
		}
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
		m.togglePause()
		return m, nil
	case "s":
		m.stopPlayer()
		return m, nil
	case "g":
		m.selectedIdx = 0
		m.scrollIdx = 0
		m.mode = "normal"
		return m, nil
	case "G":
		m.selectedIdx = len(m.videos) - 1
		m.fixScroll()
		return m, nil
	case "l", "enter", " ":
		if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
	case "h", "P":
		m.showPlaylists = true
		m.selectedIdx = 0
		m.scrollIdx = 0
		playlists, _ := m.store.ListPlaylists()
		m.playlists = playlists
		return m, nil
	}

	if m.showPlaylists {
		return m.handlePlaylistKey(msg)
	}

	return m, nil
}

func (m *Model) fixScroll() {
	itemHeight := 2
	if m.showPlaylists {
		itemHeight = 1
	}

	maxItems := (m.height - 15) / itemHeight
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

func (m *Model) handlePlaylistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.selectedIdx < len(m.playlists)+2 {
			if m.selectedIdx == 0 {
				m.view = "history"
				m.showPlaylists = false
				return m, m.loadHistory()
			} else if m.selectedIdx == 1 {
				m.showPlaylists = false
				return m, nil
			}
			idx := m.selectedIdx - 2
			if idx < len(m.playlists) {
				playlistName := m.playlists[idx]
				videos, _ := m.store.GetPlaylist(playlistName)
				m.videos = videos
				m.selectedIdx = 0
				m.scrollIdx = 0
				m.mode = "normal"
				m.showPlaylists = false
				m.view = "playlist"
			}
		}
	case "j", "down":
		if m.selectedIdx < len(m.playlists)+1 {
			m.selectedIdx++
			m.fixScroll()
		}
	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.fixScroll()
		}
	case "h", "q":
		m.showPlaylists = false
		m.selectedIdx = 0
		m.scrollIdx = 0
		m.mode = "normal"
	}
	return m, nil
}

func (m *Model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	if m.showPlaylists {
		return m.playlistsView()
	}

	var mainContent string

	if m.mode == "insert" {
		mainContent = m.searchInput.View()
		if len(m.videos) > 0 {
			mainContent = lipgloss.JoinVertical(
				lipgloss.Left,
				mainContent,
				m.videoListView(),
			)
		}
	} else if m.loading {
		mainContent = statusStyle.Render("» " + m.loadingText)
	} else if len(m.videos) == 0 {
		mainContent = normalStyle.Render("No videos found")
	} else {
		mainContent = m.videoListView()
	}

	details := m.detailsView()

	layout := lipgloss.JoinVertical(
		lipgloss.Left,
		mainContent,
		details,
	)

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gray).
		Padding(0, 1)

	if m.width > 0 {
		border = border.Width(m.width - 4)
	}

	content := border.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("YouTube TUI"),
			layout,
		),
	)

	var statusBar string
	if m.nowPlaying != "" {
		status := "▶ Playing: "
		if m.isPaused {
			status = "⏸ Paused: "
		}
		statusBar = lipgloss.NewStyle().Foreground(green).Render(status) + normalStyle.Render(m.nowPlaying)
	} else if m.loading {
		statusBar = statusStyle.Render("» " + m.loadingText)
	} else {
		modeStr := "-- NORMAL --"
		if m.mode == "insert" {
			modeStr = "-- INSERT --"
		}
		statusBar = normalStyle.Render(modeStr + " j/k: navigate  p: pause  s: stop  ?: help  esc: quit")
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		statusBar,
	)
}

func (m *Model) helpView() string {
	help := `
  KEYBINDINGS
  -----------
  j/down:      Move down
  k/up:        Move up
  g:           Go to top
  G:           Go to bottom
  l/Enter/Spc: Play video
  p:           Pause/Resume
  s:           Stop current song
  i/ /:        Search mode
  h/P:         Playlists / History
  ?:           Show/hide help
  q:           Back / Normal mode
  esc/Ctrl+C:  Quit
`
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Padding(1).
		Render(help)
}

func (m *Model) videoListView() string {
	var lines []string

	// Each video takes 2 lines
	maxItems := (m.height - 15) / 2
	if maxItems < 1 {
		maxItems = 1
	}

	endIdx := m.scrollIdx + maxItems
	if endIdx > len(m.videos) {
		endIdx = len(m.videos)
	}

	for i := m.scrollIdx; i < endIdx; i++ {
		v := m.videos[i]
		title := truncate(v.Title, m.width-10)
		desc := fmt.Sprintf("%s | %s", v.Duration, v.Channel)

		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+title))
			lines = append(lines, selectedStyle.Render("  "+desc))
		} else {
			lines = append(lines, normalStyle.Render("  "+title))
			lines = append(lines, secondaryStyle.Render("  "+desc))
		}
	}

	if len(lines) == 0 {
		return normalStyle.Render("No videos to display")
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *Model) detailsView() string {
	if len(m.videos) == 0 || m.selectedIdx >= len(m.videos) {
		return ""
	}
	v := m.videos[m.selectedIdx]

	width := m.width - 10
	if width < 10 {
		width = 10
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n\n%s\n%s",
		titleStyle.Render("Title:")+" "+normalStyle.Render(truncate(v.Title, width)),
		titleStyle.Render("Channel:")+" "+normalStyle.Render(truncate(v.Channel, width)),
		titleStyle.Render("Duration:")+" "+normalStyle.Render(v.Duration),
		titleStyle.Render("Views:")+" "+normalStyle.Render(v.Views),
		titleStyle.Render("Uploaded:")+" "+normalStyle.Render(v.Uploaded),
		titleStyle.Render("Description:"),
		secondaryStyle.Render(truncate(v.Description, width*3)),
	)
}

func (m *Model) playlistsView() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Playlists"))
	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("Select a playlist:"))
	lines = append(lines, "")

	items := []string{"View History", "Back"}
	items = append(items, m.playlists...)

	maxHeight := m.height - 10
	if maxHeight < 1 {
		maxHeight = 1
	}

	for i, p := range items {
		if i < m.scrollIdx || i >= m.scrollIdx+maxHeight {
			continue
		}
		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+p))
		} else {
			lines = append(lines, normalStyle.Render("  "+p))
		}
		if i >= 2 {
			idx := i - 2
			if idx < len(m.playlists) {
				videos, _ := m.store.GetPlaylist(m.playlists[idx])
				lines = append(lines, secondaryStyle.Render(fmt.Sprintf("    %d videos", len(videos))))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("q/h: back  j/k: navigate  Enter: select"))

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gray).
		Padding(1)

	if m.width > 0 {
		style = style.Width(m.width - 4)
	}

	return style.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m *Model) searchVideos(query string) tea.Cmd {
	m.loading = true
	m.loadingText = "Searching " + query + "..."
	return func() tea.Msg {
		videos, err := youtube.Search(query, m.cfg.MaxResults)
		return searchResultMsg{videos: videos, err: err}
	}
}

func (m *Model) playVideo(video youtube.Video) tea.Cmd {
	return func() tea.Msg {
		url, err := youtube.GetStreamURL(video.URL)
		if err != nil {
			return playResultMsg{url: "", title: video.Title, err: err}
		}
		url = strings.TrimSpace(url)
		return playResultMsg{url: url, title: video.Title, err: nil}
	}
}

func (m *Model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		history, err := m.store.GetHistory()
		if err != nil || len(history) == 0 {
			return searchResultMsg{videos: []youtube.Video{}, err: nil}
		}
		var videos []youtube.Video
		for _, h := range history {
			videos = append(videos, h.Video)
		}
		return searchResultMsg{videos: videos, err: nil}
	}
}

type searchResultMsg struct {
	videos []youtube.Video
	err    error
}

type playResultMsg struct {
	url   string
	title string
	err   error
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func Run(cfg *config.Config, store *storage.Storage) error {
	p := tea.NewProgram(New(cfg, store), tea.WithOutput(nil))
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
