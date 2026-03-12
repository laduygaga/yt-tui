package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
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
	cfg             *config.Config
	store           *storage.Storage
	videos          []youtube.Video
	selectedIdx     int
	mode            string
	view            string
	loading         bool
	loadingText     string
	statusMsg       string
	nowPlaying      string
	searchInput     textinput.Model
	playlists       []string
	showPlaylists   bool
	showHelp        bool
	width           int
	height          int
	scrollIdx       int
	playerCmd       *exec.Cmd
	isPaused        bool
	lastKey         string
	progress        progress.Model
	currentTime     float64
	totalTime       float64
	program         *tea.Program
	mainVideos      []youtube.Video
	mainSelected    int
	mainScroll      int
	currentPlaylist string
}

type progressMsg float64
type syncTimeMsg struct {
	Current float64
	Total   float64
}
type clearStatusMsg struct{}

func (m *Model) tickProgress() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		if m.playerCmd == nil || m.playerCmd.Process == nil || m.isPaused {
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
		progress:    progress.New(progress.WithScaledGradient("#000000", "#696c77")),
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
			m.nowPlaying = ""
			return m, nil
		}
		m.nowPlaying = msg.video.Title
		m.isPaused = false
		if msg.url != "" {
			msg.video.URL = msg.url
			m.store.AddToHistory(msg.video)
			m.currentTime = 0
			m.totalTime = parseDuration(msg.video.Duration)
		}
		return m, tea.Batch(m.startPlayer(msg.url, msg.video.Title), m.tickProgress())
	case progressMsg:
		if m.playerCmd != nil && m.playerCmd.Process != nil && !m.isPaused {
			m.currentTime += float64(msg)
			if m.currentTime > m.totalTime {
				m.currentTime = m.totalTime
			}
			return m, m.tickProgress()
		}
		return m, nil
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	return m, nil
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
	if m.playerCmd != nil && m.playerCmd.Process != nil {
		m.playerCmd.Process.Kill()
	}

	socketPath := "/tmp/yt-tui-mpv.sock"
	m.playerCmd = exec.Command(m.cfg.Player, "--no-video", "--quiet", fmt.Sprintf("--input-ipc-server=%s", socketPath), url)
	err := m.playerCmd.Start()
	if err != nil {
		m.nowPlaying = "Error: " + err.Error()
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		for {
			if m.playerCmd == nil || m.playerCmd.Process == nil || m.isPaused {
				time.Sleep(1 * time.Second)
				continue
			}

			cmdStr := `{"command":["get_property","time-pos"]}`
			out, err := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc -w 1 -U %s", cmdStr, socketPath)).Output()
			if err == nil {
				var resp struct {
					Data  float64 `json:"data"`
					Error string  `json:"error"`
				}
				if jsonErr := json.Unmarshal(out, &resp); jsonErr == nil && resp.Error == "success" {
					currentTime := resp.Data

					durCmd := `{"command":["get_property","duration"]}`
					durOut, durErr := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc -w 1 -U %s", durCmd, socketPath)).Output()
					totalTime := 0.0
					if durErr == nil {
						var durResp struct {
							Data  float64 `json:"data"`
							Error string  `json:"error"`
						}
						if jErr := json.Unmarshal(durOut, &durResp); jErr == nil && durResp.Error == "success" {
							totalTime = durResp.Data
						}
					}

					if m.program != nil {
						m.program.Send(syncTimeMsg{Current: currentTime, Total: totalTime})
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

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
	if m.playerCmd == nil || m.playerCmd.Process == nil {
		return
	}

	m.isPaused = !m.isPaused
	pauseVal := "false"
	if m.isPaused {
		pauseVal = "true"
	}

	go func() {
		cmdStr := fmt.Sprintf(`{"command":["set_property","pause",%s]}`, pauseVal)
		exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc -w 1 -U /tmp/yt-tui-mpv.sock", cmdStr)).Run()
	}()
}

func (m *Model) seek(seconds float64) {
	if m.playerCmd == nil || m.playerCmd.Process == nil {
		return
	}

	m.currentTime += seconds
	if m.currentTime < 0 {
		m.currentTime = 0
	}
	if m.currentTime > m.totalTime {
		m.currentTime = m.totalTime
	}

	go func() {
		cmdStr := fmt.Sprintf(`{"command":["seek",%f,"relative"]}`, seconds)
		exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc -w 1 -U /tmp/yt-tui-mpv.sock", cmdStr)).Run()
	}()
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	if m.showPlaylists {
		return m.handlePlaylistKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.stopPlayer()
		return m, tea.Quit
	case "ctrl+l":
		if m.view == "history" {
			m.store.ClearHistory()
			m.statusMsg = "History cleared"
			return m, tea.Batch(
				m.loadHistory(),
				tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
					return clearStatusMsg{}
				}),
			)
		}
		if m.view == "playlist" && m.currentPlaylist == "favorit" {
			m.store.ClearPlaylist("favorit")
			m.videos = []youtube.Video{}
			m.statusMsg = "Favorit cleared"
			return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, nil
	case "esc", "q":
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
		m.stopPlayer()
		return m, tea.Quit
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
		m.togglePause()
		return m, nil
	case "h":
		if m.nowPlaying != "" {
			m.seek(-10)
		}
		return m, nil
	case "l":
		if m.nowPlaying != "" {
			m.seek(10)
		} else if len(m.videos) > 0 && m.selectedIdx < len(m.videos) {
			video := m.videos[m.selectedIdx]
			m.loading = true
			m.loadingText = "Getting stream..."
			return m, m.playVideo(video)
		}
		return m, nil
	case "s":
		m.stopPlayer()
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

				return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
					return clearStatusMsg{}
				})
			}
		}
		return m, nil
	case "H":
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
	}

	return m, nil
}

func (m *Model) fixScroll() {
	itemHeight := 2
	if m.showPlaylists {
		itemHeight = 1
	}

	offset := 15
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

func (m *Model) handlePlaylistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.selectedIdx < len(m.playlists)+2 {
			if m.selectedIdx == 0 {
				m.view = "history"
				m.videos = []youtube.Video{}
				m.showPlaylists = false
				m.loading = true
				m.loadingText = "Loading history..."
				return m, m.loadHistory()
			} else if m.selectedIdx == 1 {
				m.showPlaylists = false
				return m, nil
			}
			idx := m.selectedIdx - 2
			if idx < len(m.playlists) {
				playlistName := m.playlists[idx]
				m.currentPlaylist = playlistName
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

func (m *Model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	if m.showPlaylists {
		return m.playlistsView()
	}

	var mainContent string
	var details string

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
		details = m.detailsView()
	}

	layout := mainContent
	if details != "" {
		layout = lipgloss.JoinVertical(
			lipgloss.Left,
			mainContent,
			details,
		)
	}

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
			titleStyle.Render("YouTube TUI"+getTitleSuffix(m)),
			layout,
		),
	)

	var statusBar string
	if m.nowPlaying != "" {
		status := "▶ Playing: "
		if m.isPaused {
			status = "⏸ Paused: "
		}

		var progressStr string
		if m.totalTime > 0 {
			pct := m.currentTime / m.totalTime
			m.progress.Width = m.width - 20
			if m.progress.Width < 20 {
				m.progress.Width = 20
			}
			progressStr = "\n" + m.progress.ViewAs(pct) + fmt.Sprintf(" %s / %s", formatTime(m.currentTime), formatTime(m.totalTime))
		} else if m.currentTime > 0 {
			progressStr = "\n" + fmt.Sprintf(" %s / --:-- (Loading duration...)", formatTime(m.currentTime))
		}

		statusBar = lipgloss.NewStyle().Foreground(green).Render(status) + normalStyle.Render(m.nowPlaying) + progressStr
	} else if m.loading {
		statusBar = statusStyle.Render("» " + m.loadingText)
	} else {
		modeStr := "-- NORMAL --"
		if m.mode == "insert" {
			modeStr = "-- INSERT --"
		}
		statusBar = normalStyle.Render(modeStr + " j/k: navigate  h/l: seek  p: pause  s: stop  ?: help  esc: quit")
	}

	if m.statusMsg != "" {
		statusBar = statusBar + "\n" + lipgloss.NewStyle().Foreground(yellow).Render("» "+m.statusMsg)
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
  h/l:         Seek backward/forward 10s
  j/down:      Move down
  k/up:        Move up
  g:           Go to top
  G:           Go to bottom
  Enter/Spc:   Play video
  p:           Pause/Resume
  s:           Stop current song
  *:           Toggle Favorite
  dd:          Delete item
  Ctrl+L:      Clear list (History/Favorit)
  i/ /:        Search mode
  H/P:         Playlists / History
  ?:           Show/hide help
  q/esc:       Back / Normal mode / Quit
  Ctrl+C:      Force Quit
`
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Padding(1).
		Render(help)
}

func (m *Model) videoListView() string {
	if len(m.videos) == 0 {
		return normalStyle.Render("No videos in current list")
	}

	var lines []string

	offset := 15
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	// Each video takes 2 lines
	maxItems := (m.height - offset) / 2
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

	offset := 15
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	maxHeight := m.height - offset
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
		url, err := youtube.GetStreamURL(video.URL)
		if err != nil {
			return playResultMsg{url: "", video: video, err: err}
		}
		url = strings.TrimSpace(url)
		return playResultMsg{url: url, video: video, err: nil}
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

type searchResultMsg struct {
	videos []youtube.Video
	err    error
}

type playResultMsg struct {
	url   string
	video youtube.Video
	err   error
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

func Run(cfg *config.Config, store *storage.Storage) error {
	m := New(cfg, store)
	p := tea.NewProgram(m, tea.WithOutput(nil))
	m.program = p
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
