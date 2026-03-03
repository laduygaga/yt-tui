package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
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
	case tea.KeyMsg:
		return m.handleKey(msg)
	case searchResultMsg:
		m.loading = false
		m.videos = msg.videos
		m.selectedIdx = 0
		m.mode = "normal"
		return m, nil
	case playResultMsg:
		m.loading = false
		if msg.err != nil {
			m.nowPlaying = ""
			return m, nil
		}
		m.nowPlaying = msg.title
		go m.runPlayer(msg.url, msg.title)
		return m, nil
	}
	return m, nil
}

func (m *Model) runPlayer(url, title string) {
	cmd := exec.Command(m.cfg.Player, "--no-video", "--quiet", url)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Start()
	cmd.Wait()

	video := youtube.Video{Title: title}
	m.store.AddToHistory(video)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
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
		}
		return m, nil
	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
		return m, nil
	case "g":
		m.selectedIdx = 0
		m.mode = "normal"
		return m, nil
	case "G":
		m.selectedIdx = len(m.videos) - 1
		return m, nil
	case "l", "enter", " ":
		if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
	case "h", "p", "P":
		m.showPlaylists = true
		playlists, _ := m.store.ListPlaylists()
		m.playlists = playlists
		return m, nil
	}

	if m.showPlaylists {
		return m.handlePlaylistKey(msg)
	}

	return m, nil
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
		m.mode = "normal"
				m.showPlaylists = false
				m.view = "playlist"
			}
		}
	case "j", "down":
		if m.selectedIdx < len(m.playlists)+1 {
			m.selectedIdx++
		}
	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "h", "esc":
		m.showPlaylists = false
		m.selectedIdx = 0
		m.mode = "normal"
	}
	return m, nil
}

func (m *Model) View() string {
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

	content := border.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("YouTube TUI"),
			layout,
		),
	)

	var statusBar string
	if m.nowPlaying != "" {
		statusBar = lipgloss.NewStyle().Foreground(green).Render("▶ Playing: ") + normalStyle.Render(m.nowPlaying)
	} else if m.loading {
		statusBar = statusStyle.Render("» " + m.loadingText)
	} else {
		modeStr := "-- NORMAL --"
		if m.mode == "insert" {
			modeStr = "-- INSERT --"
		}
		statusBar = normalStyle.Render(modeStr + " j/k: down/up  h: playlists  l/Enter/Space: play  i: search  Esc: close  q: quit")
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		statusBar,
	)
}

func (m *Model) videoListView() string {
	var lines []string
	for i, v := range m.videos {
		title := v.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		desc := fmt.Sprintf("%s | %s", v.Duration, v.Channel)

		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+title))
			lines = append(lines, selectedStyle.Render("  "+desc))
		} else {
			lines = append(lines, normalStyle.Render("  "+title))
			lines = append(lines, secondaryStyle.Render("  "+desc))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *Model) detailsView() string {
	if len(m.videos) == 0 || m.selectedIdx >= len(m.videos) {
		return ""
	}
	v := m.videos[m.selectedIdx]
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n\n%s\n%s",
		titleStyle.Render("Title:")+" "+normalStyle.Render(truncate(v.Title, 60)),
		titleStyle.Render("Channel:")+" "+normalStyle.Render(v.Channel),
		titleStyle.Render("Duration:")+" "+normalStyle.Render(v.Duration),
		titleStyle.Render("Views:")+" "+normalStyle.Render(v.Views),
		titleStyle.Render("Uploaded:")+" "+normalStyle.Render(v.Uploaded),
		titleStyle.Render("Description:"),
		secondaryStyle.Render(truncate(v.Description, 200)),
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

	for i, p := range items {
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
	lines = append(lines, secondaryStyle.Render("Esc/h: back  j/k: navigate  Enter: select"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gray).
		Padding(1).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
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
