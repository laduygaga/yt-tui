package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	helpStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyan).
			Padding(1)

	transcriptStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyan).
			Padding(1)

	playlistBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gray).
			Padding(1)

	mainBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gray).
			Padding(0, 1)

	yellowStyle = lipgloss.NewStyle().Foreground(yellow)
	greenStyle  = lipgloss.NewStyle().Foreground(green)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cyanAlignCenterWidth = lipgloss.NewStyle().
				Foreground(cyan).
				Align(lipgloss.Center)
	dimAlignCenterWidth = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Align(lipgloss.Center)
)

func (m *Model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	if m.showPlaylists {
		return m.playlistsView()
	}
	if m.showTranscriptView {
		return m.transcriptView()
	}

	content := m.mainContent()

	var statusBar string
	isPlayerActive := m.player.IsPlaying()
	if (isPlayerActive || m.totalTime > 0) && m.nowPlaying != "" {
		statusBar = m.playbackStatusBar(isPlayerActive)
	} else if m.loading {
		statusBar = statusStyle.Render("» " + m.loadingText)
	} else {
		modeStr := "-- NORMAL --"
		if m.mode == "insert" {
			modeStr = "-- INSERT --"
		}
		statusBar = normalStyle.Render(modeStr + " j/k: navigate  h/l: seek  [/]: speed  p: pause  s: stop  ?: help  esc: quit")
	}

	if m.statusMsg != "" {
		statusBar = statusBar + "\n" + yellowStyle.Render("» "+m.statusMsg)
	}

	if m.nowPlaying != "" && m.showSubtitles {
		subtitle := m.subtitleSection()
		return lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			subtitle,
			"",
			statusBar,
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		"",
		statusBar,
	)
}

func (m *Model) mainContent() string {
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

	if details == "" {
		return m.renderBordered(mainContent)
	}
	return m.renderBordered(lipgloss.JoinVertical(lipgloss.Left, mainContent, details))
}

func (m *Model) ensureWidthStyles() {
	width := m.width - borderPadding
	if width < 10 {
		width = 10
	}
	if m.cachedStyleWidth == width {
		return
	}
	m.cachedStyleWidth = width
	m.cachedMainBorder = mainBorder.Width(width)
	m.cachedPlaylistBorder = playlistBorder.Width(width)
	m.cachedDimStyle = dimAlignCenterWidth.Width(width)
	m.cachedCyanStyle = cyanAlignCenterWidth.Width(width)
}

func (m *Model) renderBordered(content string) string {
	m.ensureWidthStyles()
	return m.cachedMainBorder.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("YouTube TUI"+getTitleSuffix(m)),
			content,
		),
	)
}

func (m *Model) playbackStatusBar(isPlayerActive bool) string {
	snap := m.player.Snapshot()
	status := "▶ Playing: "
	if !isPlayerActive && m.totalTime > 0 {
		status = "■ Stopped: "
	} else if snap.IsPaused {
		status = "⏸ Paused: "
	}
	if snap.IsLooping {
		status += "🔁 "
	}

	progressStr := m.progressStr()
	return greenStyle.Render(status) + normalStyle.Render(m.nowPlaying) + progressStr
}

func (m *Model) progressStr() string {
	if m.totalTime > 0 {
		pct := m.currentTime / m.totalTime
		if pct > 1 {
			pct = 1
		}
		width := m.width - 20
		if width < 20 {
			width = 20
		}
		if m.progressWidthSet != width {
			m.progress.Width = width
			m.progressWidthSet = width
		}
		timeStr := fmt.Sprintf(" %s / %s", formatTime(m.currentTime), formatTime(m.totalTime))
		if m.playbackSpeed != defaultSpeed {
			timeStr += fmt.Sprintf(" [%.2fx]", m.playbackSpeed)
		}
		return "\n" + m.progress.ViewAs(pct) + timeStr
	}
	if m.currentTime > 0 {
		timeStr := fmt.Sprintf(" %s / --:-- (Loading duration...)", formatTime(m.currentTime))
		if m.playbackSpeed != defaultSpeed {
			timeStr += fmt.Sprintf(" [%.2fx]", m.playbackSpeed)
		}
		return "\n" + timeStr
	}
	return ""
}

func (m *Model) subtitleSection() string {
	m.ensureWidthStyles()
	if m.transcript == nil {
		return m.cachedDimStyle.Render("[Loading subtitles...]")
	}
	if len(m.transcript.Lines) == 0 {
		return m.cachedDimStyle.Render("[No subtitles available]")
	}
	primary, secondary := m.getCurrentSubtitle()
	if primary == "" && secondary == "" {
		return " "
	}

	maxLen := m.width - 10
	if maxLen < 10 {
		maxLen = 10
	}

	if primary != "" {
		if len(primary) > maxLen {
			primary = primary[:maxLen-3] + "..."
		}
		primary = "▼ " + primary
	}

	if secondary != "" {
		if len(secondary) > maxLen {
			secondary = secondary[:maxLen-3] + "..."
		}
		secondary = "  " + secondary
		return m.cachedCyanStyle.Render(primary + "\n" + secondary)
	}

	return m.cachedCyanStyle.Render(primary)
}

func (m *Model) transcriptView() string {
	if m.transcript == nil || len(m.transcript.Lines) == 0 {
		return normalStyle.Render("No transcript available")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("TRANSCRIPT (j/k to scroll, t to close)"))
	lines = append(lines, "")

	maxLines := m.height - 5
	if maxLines < 1 {
		maxLines = 1
	}

	endIdx := m.transcriptScrollIdx + maxLines
	if endIdx > len(m.transcript.Lines) {
		endIdx = len(m.transcript.Lines)
	}

	for i := m.transcriptScrollIdx; i < endIdx; i++ {
		line := m.transcript.Lines[i]
		timeStr := formatTime(line.Start)
		text := line.Text
		if line.SecondaryText != "" {
			text = text + " (" + line.SecondaryText + ")"
		}
		if len(text) > m.width-15 {
			text = text[:m.width-18] + "..."
		}

		isCurrent := m.currentTime >= (line.Start-subtitleStartOffset) && m.currentTime < (line.Start+line.Duration+subtitleEndOffset)
		if isCurrent {
			lines = append(lines, selectedStyle.Render(fmt.Sprintf("[%s] ▶ %s", timeStr, text)))
		} else {
			lines = append(lines, normalStyle.Render(fmt.Sprintf("[%s]   %s", timeStr, text)))
		}
	}

	if endIdx < len(m.transcript.Lines) {
		lines = append(lines, secondaryStyle.Render(fmt.Sprintf("... %d more lines (j to scroll)", len(m.transcript.Lines)-endIdx)))
	}

	return transcriptStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m *Model) helpView() string {
	help := `
  KEYBINDINGS
  -----------
  h/l:         Seek backward/forward 10s
  [/]:         Decrease/Increase playback speed (0.25x steps)
  L:           Toggle loop current song
  j/down:      Move down
  k/up:        Move up
  g:           Go to top
  G:           Go to bottom
  Enter/Spc:   Play video
  p:           Pause/Resume
  s:           Stop current song
  c:           Toggle captions/subtitles (while playing)
  t:           Toggle transcript view (while playing)
  y:           Copy URL to clipboard
  *:           Toggle Favorite
  dd:          Delete item
  Ctrl+L:      Clear list (History/Favorit)
  i/ /:        Search mode
  H:           Toggle History
  P:           Toggle Playlists menu
  ?:           Show/hide help
  q/esc:       Back / Normal mode / Quit
  Ctrl+C:      Force Quit
`
	return helpStyle.Render(help)
}

func (m *Model) videoListView() string {
	if len(m.videos) == 0 {
		if m.loading {
			return statusStyle.Render("» " + m.loadingText)
		}
		return normalStyle.Render("No videos in current list")
	}

	m.ensureTruncationCache()

	var content strings.Builder

	offset := uiOffsetBase
	if m.height < offset+2 {
		offset = m.height - 2
		if offset < 0 {
			offset = 0
		}
	}

	maxItems := (m.height - offset) / videoItemHeight
	if maxItems < 1 {
		maxItems = 1
	}

	endIdx := m.scrollIdx + maxItems
	if endIdx > len(m.videos) {
		endIdx = len(m.videos)
	}

	for i := m.scrollIdx; i < endIdx; i++ {
		v := m.videos[i]
		title := m.cachedTitles[i]
		desc := m.formattedDur[i] + " | " + v.Channel

		if i == m.selectedIdx {
			content.WriteString(selectedStyle.Render("▶ " + title))
			content.WriteByte('\n')
			content.WriteString(selectedStyle.Render("  " + desc))
		} else {
			content.WriteString(normalStyle.Render("  " + title))
			content.WriteByte('\n')
			content.WriteString(secondaryStyle.Render("  " + desc))
		}
		if i < endIdx-1 {
			content.WriteByte('\n')
		}
	}

	if content.Len() == 0 {
		return normalStyle.Render("No videos to display")
	}

	return content.String()
}

func (m *Model) detailsView() string {
	if len(m.videos) == 0 || m.selectedIdx >= len(m.videos) {
		return ""
	}
	idx := m.selectedIdx
	v := m.videos[idx]
	m.ensureTruncationCache()

	width := m.width - 10
	if width < 10 {
		width = 10
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n\n%s\n%s",
		titleStyle.Render("Title:")+" "+normalStyle.Render(m.cachedTitles[idx]),
		titleStyle.Render("Channel:")+" "+normalStyle.Render(truncate(v.Channel, width)),
		titleStyle.Render("Duration:")+" "+normalStyle.Render(m.formattedDur[idx]),
		titleStyle.Render("Views:")+" "+normalStyle.Render(m.formattedViews[idx]),
		titleStyle.Render("Uploaded:")+" "+normalStyle.Render(v.Uploaded),
		titleStyle.Render("Description:"),
		secondaryStyle.Render(m.cachedDesc[idx]),
	)
}

func (m *Model) getCurrentSubtitle() (string, string) {
	if m.transcript == nil || len(m.transcript.Lines) == 0 {
		return "", ""
	}
	lines := m.transcript.Lines
	lo, hi := 0, len(lines)
	for lo < hi {
		mid := (lo + hi) / 2
		if m.currentTime < lines[mid].Start-subtitleStartOffset {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	idx := lo - 1
	if idx < 0 {
		return "", ""
	}
	line := lines[idx]
	startTime := line.Start - subtitleStartOffset
	endTime := line.Start + line.Duration + subtitleEndOffset
	if m.currentTime >= startTime && m.currentTime < endTime {
		return line.Text, line.SecondaryText
	}
	return "", ""
}

func (m *Model) playlistsView() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Playlists"))
	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("Select a playlist:"))
	lines = append(lines, "")

	offset := uiOffsetBase
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

	for i, p := range m.playlists {
		if i < m.scrollIdx || i >= m.scrollIdx+maxHeight {
			continue
		}
		if i == m.selectedIdx {
			lines = append(lines, selectedStyle.Render("▶ "+p))
		} else {
			lines = append(lines, normalStyle.Render("  "+p))
		}
		videos, _ := m.store.GetPlaylist(p)
		lines = append(lines, secondaryStyle.Render(fmt.Sprintf("    %d videos", len(videos))))
	}

	lines = append(lines, "")
	lines = append(lines, secondaryStyle.Render("P/q/h/esc: back  j/k: navigate  Enter: select"))

	m.ensureWidthStyles()
	return m.cachedPlaylistBorder.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
